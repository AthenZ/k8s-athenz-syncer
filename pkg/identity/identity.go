/*
Copyright 2019, Oath Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package identity

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/yahoo/athenz/libs/go/zmssvctoken"
	"github.com/yahoo/k8s-athenz-syncer/pkg/clusterconfig"
	"github.com/yahoo/k8s-athenz-syncer/pkg/crypto"
)

var (
	gracePeriod   = 5 * time.Minute
	defaultExpiry = 24 * time.Hour
)

// PrivateKeyProvider returns a signing key on demand.
type PrivateKeyProvider func() (*crypto.SigningKey, error)

// tokenProvider provides tokens.
type tokenProvider interface {
	Token() (string, error)
}

type x509Provider interface {
	X509Cert() (*tls.Config, error)
}

type X509Provider struct {
	once         sync.Once
	stop         chan struct{}
	l            sync.RWMutex
	current      *tls.Config
	certFilePath string
	keyFilePath  string
	caCertFile   string
	expiry       time.Time
}

// IdentityProvider provides ntokens and optional TLS certs.
type IdentityProvider struct {
	tokenProvider
	x509Provider
	domain  string
	service string
	podIP   net.IP
	cc      *clusterconfig.ClusterConfiguration
	ks      PrivateKeyProvider
}

// tp implements tokenProvider
type tp struct {
	domain  string
	service string
	ip      string
	host    string
	expiry  time.Duration
	ks      PrivateKeyProvider
	l       sync.Mutex
	current string
	expire  time.Time
}

// Config is the identity provider configuration.
type Config struct {
	Cluster                   *clusterconfig.ClusterConfiguration // cluster config
	Domain                    string                              // Athenz domain
	Service                   string                              // Athenz service
	PodIP                     net.IP                              // Pod IP for IP SAN, nil ok for now
	PrivateKeyProvider        PrivateKeyProvider                  // source for private keys
	TokenHost                 string                              // ntoken hostname where available
	TokenIP                   net.IP                              // ntoken IP address
	TokenExpiry               time.Duration                       // token expire time
	X509CertFile              string                              // file path for x509 cert
	X509KeyFile               string                              // file path for x509 key
	AthenzClientAuthnx509Mode bool                                // athenz client authentication. default is token.
}

// NewIdentityProvider returns an identity provider for the supplied configuration.
// It returns either x509Provider or TokenProvider based on the presence of x509 configurations.
func NewIdentityProvider(config Config) (*IdentityProvider, error) {
	if config.TokenExpiry == 0 {
		config.TokenExpiry = time.Hour
	}
	var ipStr string
	if config.TokenIP != nil {
		ipStr = config.TokenIP.String()
	}

	ip := &IdentityProvider{
		domain:  config.Domain,
		service: config.Service,
		cc:      config.Cluster,
		ks:      config.PrivateKeyProvider,
		podIP:   config.PodIP,
	}

	if config.AthenzClientAuthnx509Mode {
		if config.X509CertFile != "" && config.X509KeyFile != "" {
			x509Provider := &X509Provider{
				certFilePath: config.X509CertFile,
				keyFilePath:  config.X509KeyFile,
			}
			if _, err := x509Provider.X509Cert(); err != nil {
				return nil, err
			}
			ip.x509Provider = x509Provider
		} else {
			return nil, errors.New("cert and key both are required when AthenzClientAuthnx509Mode is set to true")
		}
	} else {
		tp := &tp{
			domain:  config.Domain,
			service: config.Service,
			expiry:  config.TokenExpiry,
			ks:      config.PrivateKeyProvider,
			host:    config.TokenHost,
			ip:      ipStr,
		}
		if _, err := tp.Token(); err != nil {
			return nil, errors.Wrap(err, "mint token")
		}
		ip.tokenProvider = tp
	}

	return ip, nil
}

// tlsConfiguration returns tls.config.
func (x *X509Provider) tlsConfiguration(cacertpem []byte) (*tls.Config, error) {
	config := &tls.Config{}
	mycert, err := tls.LoadX509KeyPair(x.certFilePath, x.keyFilePath)
	if err != nil {
		return nil, err
	}
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0] = mycert

	if cacertpem != nil {
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(cacertpem)
		config.RootCAs = certPool
	}
	return config, nil
}

// X509Cert implements x509Provider interface which returns tls.config based on
// the file path of x509Key and x509Cert
func (x *X509Provider) X509Cert() (*tls.Config, error) {
	if x.current == nil {
		err := x.updateCert()
		if err != nil {
			return nil, err
		}
		expireDuration := x.expiry.Sub(time.Now())
		if expireDuration < 0 {
			return nil, errors.New("certificate is already expired.")
		}
		log.Println("certificate expires in " + time.Duration(expireDuration).String())
		go x.refreshLoop(time.Duration(expireDuration / 3))
		return x.current, err
	}
	x.l.RLock()
	defer x.l.RUnlock()
	return x.current, nil
}

func (tp *tp) updateToken() error {
	key, err := tp.ks()
	if err != nil {
		return err
	}
	pem, err := key.PEMBytes()
	if err != nil {
		return err
	}
	b, err := zmssvctoken.NewTokenBuilder(tp.domain, tp.service, pem, key.Version)
	if err != nil {
		return err
	}
	b.SetIPAddress(tp.ip)
	b.SetHostname(tp.host)
	b.SetExpiration(tp.expiry)
	tok, err := b.Token().Value()
	if err != nil {
		return err
	}
	tp.current = tok
	tp.expire = time.Now().Add(tp.expiry).Add(-1 * gracePeriod)
	return nil
}

// Token implements the tokenProvider interface
func (tp *tp) Token() (string, error) {
	tp.l.Lock()
	defer tp.l.Unlock()
	now := time.Now().Add(-1 * gracePeriod)
	if tp.expire.Before(now) {
		if err := tp.updateToken(); err != nil {
			return "", err
		}
	}
	return tp.current, nil
}

// updateCert reads cert and keys from a file and sets
// tls.config for x509Provider
func (x *X509Provider) updateCert() error {
	var cacertPem []byte
	var err error
	if x.caCertFile != "" {
		cacertPem, err = ioutil.ReadFile(x.caCertFile)
		if err != nil {
			return err
		}
	}
	tlsConfig, err := x.tlsConfiguration(cacertPem)
	if err != nil {
		return err
	}
	pemCertBlock := pem.Block{
		Bytes: tlsConfig.Certificates[0].Certificate[0],
	}
	x.l.Lock()
	x.expiry = certExpiry(pem.EncodeToMemory(&pemCertBlock))
	defer x.l.Unlock()
	x.current = tlsConfig
	return nil
}

// refreshLoop executes on a given interval and reloads the cert and keys.
func (x *X509Provider) refreshLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := x.updateCert(); err != nil {
				log.Println("update cert error", err)
			}
		case <-x.stop:
			log.Println("stopping channel for x509Provider")
			return
		}
	}
}

// certExpiry returns the expiration of the supplied cert only if it is
// less than the default expiry. Otherwise, it returns the default expiry.
func certExpiry(certPEM []byte) time.Time {
	def := time.Now().Add(defaultExpiry)
	block, _ := pem.Decode(certPEM)
	if block == nil {
		log.Println("unable to get cert expiry, failed to decode pem block")
		return def
	}
	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		log.Println("unable to get cert expiry, failed to parse cert", err)
		return def
	}

	if x509Cert.NotAfter.After(def) {
		return def
	}
	return x509Cert.NotAfter
}
