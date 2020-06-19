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
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/yahoo/athenz/libs/go/zmssvctoken"
	"github.com/yahoo/k8s-athenz-syncer/pkg/crypto"
)

var (
	gracePeriod = 5 * time.Minute
)

// PrivateKeyProvider returns a signing key on demand.
type PrivateKeyProvider func() (*crypto.SigningKey, error)

// tokenProvider provides tokens.
type tokenProvider interface {
	Token() (string, error)
}

// IdentityProvider provides ntokens and optional TLS certs.
type IdentityProvider struct {
	tokenProvider
	domain  string
	service string
	podIP   net.IP
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
	Domain                    string             // Athenz domain
	Service                   string             // Athenz service
	PodIP                     net.IP             // Pod IP for IP SAN, nil ok for now
	PrivateKeyProvider        PrivateKeyProvider // source for private keys
	TokenHost                 string             // ntoken hostname where available
	TokenIP                   net.IP             // ntoken IP address
	TokenExpiry               time.Duration      // token expire time
	X509CertFile              string             // file path for x509 cert
	X509KeyFile               string             // file path for x509 key
	AthenzClientAuthnx509Mode bool               // athenz client authentication. default is token.
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
		ks:      config.PrivateKeyProvider,
		podIP:   config.PodIP,
	}

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

	return ip, nil
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
