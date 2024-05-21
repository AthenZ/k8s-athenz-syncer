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
package test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"

	"github.com/AthenZ/k8s-athenz-syncer/pkg/log"
)

const (
	secretName = "secret-key"
	keyFile    = "secret-key.v0"
)

func savePEMKey(fileName string, key *rsa.PrivateKey) {
	keyOut, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("failed to open %s for writing: %s", fileName, err)
	}
	defer keyOut.Close()

	var privateKey = &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	if err := pem.Encode(keyOut, privateKey); err != nil {
		log.Fatalf("failed to write data to %s: %s", fileName, err)
	}
}

func CreateKeyFile(dir string) []byte {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		log.Fatalf("Failed to generate private key. Error: %s", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, os.ModePerm)
	}
	savePEMKey(dir+keyFile, priv)
	keybytes, err := ioutil.ReadFile(dir + keyFile)
	if err != nil {
		log.Fatalf("Failed to read generated private key: %s", err)
	}
	return keybytes
}