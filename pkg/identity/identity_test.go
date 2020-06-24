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
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/yahoo/athenz/clients/go/zms"
	"github.com/yahoo/k8s-athenz-syncer/pkg/crypto"
	"github.com/yahoo/k8s-athenz-syncer/pkg/log"
	"github.com/yahoo/k8s-athenz-syncer/pkg/test"
)

const (
	domainName     = "home.domain"
	serviceName    = "test.service"
	identityKeyDir = "./tmp/"
	secretName     = "secret-key"
	keyFile        = "secret-key.v0"
)

// createTokenProvider - tokenProvider for testing
func createTokenProvider() *TokenProvider {
	log.InitLogger("/tmp/log/test.log", "info")

	test.CreateKeyFile(identityKeyDir)
	defer os.RemoveAll(identityKeyDir)

	stop := make(chan struct{})
	privateKeySource := crypto.NewPrivateKeySource(identityKeyDir, secretName)
	zmsClient := zms.NewClient("https://zms.athenz.com", &http.Transport{})
	config := Config{
		Client:             &zmsClient,
		Domain:             domainName,
		Service:            serviceName,
		PrivateKeyProvider: privateKeySource.SigningKey,
	}
	tp, err := NewTokenProvider(config, stop)
	if err != nil {
		log.Errorf("Unable to create token provider. Error: %v", err)
	}
	return tp
}

// TestToken: token should not update if token is not expired
func TestToken(t *testing.T) {
	tp := createTokenProvider()

	test.CreateKeyFile(identityKeyDir)
	defer os.RemoveAll(identityKeyDir)

	token1, err := tp.Token()
	if err != nil {
		t.Errorf("Unable to get token. Error: %v", err)
	}
	token1Expire := tp.expire
	if token1Expire.Before(time.Now().Add(50*time.Minute)) || token1Expire.After(time.Now().Add(time.Hour)) {
		t.Error("Wrong expiration time")
	}
	token2, err := tp.Token()
	if err != nil {
		t.Errorf("Unable to get token. Error: %v", err)
	}
	token2Expire := tp.expire
	if token1 != token2 || token1Expire != token2Expire {
		t.Error("Token updated when not expired")
	}
	if *tp.config.Client.CredsToken != token2 {
		t.Error("Failed to update client token")
	}
}

// TestUpdateToken: token should update everytime UpdateToken() is called
func TestUpdateToken(t *testing.T) {
	tp := createTokenProvider()

	test.CreateKeyFile(identityKeyDir)
	defer os.RemoveAll(identityKeyDir)

	err := tp.UpdateToken()
	if err != nil {
		t.Errorf("Unable to get token. Error: %v", err)
	}
	token1 := tp.current
	token1Expire := tp.expire
	if token1Expire.Before(time.Now().Add(50*time.Minute)) || token1Expire.After(time.Now().Add(time.Hour)) {
		t.Error("Token 1 wrong expiration time")
	}
	if *tp.config.Client.CredsToken != token1 {
		t.Error("Failed to update client token to token1")
	}
	err = tp.UpdateToken()
	if err != nil {
		t.Errorf("Unable to get token. Error: %v", err)
	}
	token2 := tp.current
	token2Expire := tp.expire
	if token2Expire.Before(time.Now().Add(50*time.Minute)) || token2Expire.After(time.Now().Add(time.Hour)) {
		t.Error("Token 2 wrong expiration time")
	}
	if token1 == token2 || token1Expire == token2Expire {
		t.Error("Token failed to updated")
	}
	if *tp.config.Client.CredsToken != token2 {
		t.Error("Failed to update client token to token2")
	}
}
