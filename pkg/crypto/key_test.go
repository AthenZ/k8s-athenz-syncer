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
package crypto

import (
	"os"
	"testing"

	"github.com/yahoo/k8s-athenz-syncer/pkg/log"
	"github.com/yahoo/k8s-athenz-syncer/pkg/test"
)

const (
	dirName    = "./tmp/"
	secretName = "secret-key"
	keyFile    = "secret-key.v0"
)

func pks() *PrivateKeySource {
	log.InitLogger("/tmp/log/test.log", "info")
	pks := NewPrivateKeySource(dirName, secretName)
	return pks
}

func TestSigningKey(t *testing.T) {
	pks := pks()
	test.CreateKeyFile(dirName)
	defer os.RemoveAll(dirName)
	signingKey, err := pks.SigningKey()
	if err != nil {
		t.Errorf("Failed to create signing key. Error: %v", err)
	}
	if signingKey.URI != "secret:secret-key?Version=v0" {
		t.Error("Signing Key has wrong URI")
	}
	if signingKey.Version != "v0" {
		t.Error("Wrong signing key version")
	}
	if signingKey.Value == nil {
		t.Error("Did not get private key value")
	}
}

func TestPrivateKeyFromPEMBytes(t *testing.T) {
	pks := pks()
	test.CreateKeyFile(dirName)
	defer os.RemoveAll(dirName)
	signingKey, err := pks.SigningKey()
	if err != nil {
		t.Error("Failed to create signing key")
	}
	pem, err := signingKey.PEMBytes()
	if err != nil {
		t.Errorf("Failed to create PEMBytes from signing key. Error: %v", err)
	}
	if len(pem) == 0 {
		t.Error("Invalid pem array")
	}
}

func TestPEMBytes(t *testing.T) {
	pks := pks()
	test.CreateKeyFile(dirName)
	defer os.RemoveAll(dirName)
	signingKey, err := pks.SigningKey()
	if err != nil {
		t.Error("Failed to create signing key")
	}
	pem, err := signingKey.PEMBytes()
	if err != nil {
		t.Errorf("Failed to create PEMBytes from signing key. Error: %v", err)
	}
	keyType, signer, err := PrivateKeyFromPEMBytes(pem)
	if keyType != RSA {
		t.Error("Wrong keyType")
	}
	if signer == nil {
		t.Error("Invalid signer")
	}
	if err != nil {
		t.Errorf("Unable to get private key information. Error: %v", err)
	}
}
