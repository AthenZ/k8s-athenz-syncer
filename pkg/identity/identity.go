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
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/yahoo/athenz/clients/go/zms"
	"github.com/yahoo/athenz/libs/go/zmssvctoken"
	"github.com/yahoo/k8s-athenz-syncer/pkg/crypto"
	"github.com/yahoo/k8s-athenz-syncer/pkg/log"
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

// TokenProvider implements tokenProvider provides nTokens
type TokenProvider struct {
	client  *zms.ZMSClient
	header  string
	domain  string
	service string
	expiry  time.Duration
	ks      PrivateKeyProvider
	l       sync.RWMutex
	current string
	expire  time.Time
}

// Config is the token provider configuration.
type Config struct {
	Client             *zms.ZMSClient
	Header             string
	Domain             string             // Athenz domain
	Service            string             // Athenz service
	PrivateKeyProvider PrivateKeyProvider // source for private keys
	TokenExpiry        time.Duration      // token expire time
}

// NewTokenProvider returns a token for the supplied configuration.
func NewTokenProvider(config Config, stopCh <-chan struct{}) (*TokenProvider, error) {
	if config.TokenExpiry == 0 {
		config.TokenExpiry = time.Hour
	}

	tp := &TokenProvider{
		client:  config.Client,
		header:  config.Header,
		domain:  config.Domain,
		service: config.Service,
		expiry:  config.TokenExpiry,
		ks:      config.PrivateKeyProvider,
	}
	if _, err := tp.Token(); err != nil {
		return nil, errors.Wrap(err, "mint token")
	}
	go tp.refreshLoop(time.Duration(config.TokenExpiry/3), stopCh)
	return tp, nil
}

// updateToken - creates new nToken after there was no token or the existing token has expired
func (tp *TokenProvider) updateToken() error {
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
	b.SetExpiration(tp.expiry)
	tok, err := b.Token().Value()
	if err != nil {
		return err
	}
	tp.current = tok
	tp.expire = time.Now().Add(tp.expiry).Add(-1 * gracePeriod)
	tp.client.AddCredentials(tp.header, tp.current)
	log.Info("New nToken generated and added to zmsClient credentials")
	return nil
}

// Token implements the tokenProvider interface
func (tp *TokenProvider) Token() (string, error) {
	tp.l.Lock()
	defer tp.l.Unlock()
	now := time.Now().Add(gracePeriod)
	if tp.expire.Before(now) {
		log.Info("Current NToken expired, getting ready to refresh")
		if err := tp.updateToken(); err != nil {
			return "", err
		}
	}
	return tp.current, nil
}

// refreshLoop go subroutine that checks if the current token is expired and updates the n token
func (tp *TokenProvider) refreshLoop(interval time.Duration, stopCh <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if _, err := tp.Token(); err != nil {
				log.Println("update token error", err)
			}
		case <-stopCh:
			log.Println("stopping channel for token provider")
			return
		}
	}
}
