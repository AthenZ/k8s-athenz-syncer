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

	"github.com/AthenZ/athenz/clients/go/zms"
	"github.com/AthenZ/athenz/libs/go/zmssvctoken"
	"github.com/AthenZ/k8s-athenz-syncer/pkg/crypto"
	"github.com/AthenZ/k8s-athenz-syncer/pkg/log"
	"github.com/pkg/errors"
)

const (
	refreshIntervalFactor      = 10
	expirationCheckGracePeriod = 0.25
)

// PrivateKeyProvider returns a signing key on demand.
type PrivateKeyProvider func() (*crypto.SigningKey, error)

// TokenProvider implements tokenProvider provides nTokens
type TokenProvider struct {
	config  Config
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
		config: config,
	}
	if _, err := tp.Token(); err != nil {
		return nil, errors.Wrap(err, "mint token")
	}
	go tp.refreshLoop(time.Duration(config.TokenExpiry/refreshIntervalFactor), stopCh)
	return tp, nil
}

// UpdateToken - creates new nToken after there was no token or the existing token has expired
func (tp *TokenProvider) UpdateToken() error {
	key, err := tp.config.PrivateKeyProvider()
	if err != nil {
		return err
	}
	pem, err := key.PEMBytes()
	if err != nil {
		return err
	}
	b, err := zmssvctoken.NewTokenBuilder(tp.config.Domain, tp.config.Service, pem, key.Version)
	if err != nil {
		return err
	}
	b.SetExpiration(tp.config.TokenExpiry)
	tok, err := b.Token().Value()
	if err != nil {
		return err
	}
	tp.current = tok
	tp.expire = time.Now().Add(tp.config.TokenExpiry)
	tp.config.Client.AddCredentials(tp.config.Header, tp.current)
	log.Info("New nToken generated and added to zmsClient credentials")
	log.Infof("Current nToken expiration time: %v", tp.expire)
	return nil
}

// Token implements the tokenProvider interface
func (tp *TokenProvider) Token() (string, error) {
	tp.l.Lock()
	defer tp.l.Unlock()
	gracePeriod := tp.config.TokenExpiry.Seconds() * expirationCheckGracePeriod
	now := time.Now().Add(time.Second * time.Duration(gracePeriod))
	if tp.expire.Before(now) {
		log.Info("Current NToken expired, getting ready to refresh")
		if err := tp.UpdateToken(); err != nil {
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
				log.Errorf("update token error: %v", err)
			}
		case <-stopCh:
			log.Println("stopping channel for token provider")
			return
		}
	}
}