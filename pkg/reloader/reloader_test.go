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
package reloader

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io/ioutil"
	"math/big"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/AthenZ/k8s-athenz-syncer/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/tevino/abool"
)

var (
	certFile = "cert.pem"
	keyFile  = "key.pem"
)

type Original struct {
	originalKey  []byte
	originalCert []byte
}

func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

func savePEMKey(fileName string, key *rsa.PrivateKey) {
	keyOut, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("failed to open %s for writing: %s", fileName, err)
	}
	defer keyOut.Close()

	var privateKey = &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	if err := pem.Encode(keyOut, privateKey); err != nil {
		log.Fatalf("failed to write data to %s: %s", fileName, err)
	}
}

func savePEMCert(fileName string, derBytes []byte) {
	certOut, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("failed to open %s for writing: %s", fileName, err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		log.Fatalf("failed to write data to %s: %s", fileName, err)
	}
}

func createCertAndKeyFile() ([]byte, []byte) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		log.Fatalf("Failed to generate private key. Error: %s", err)
	}
	savePEMKey("key.pem", priv)

	notBefore := time.Now()
	notAfter := notBefore.Add(time.Hour)
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(priv), priv)
	if err != nil {
		log.Fatalf("Failed to create certificate: %s", err)
	}

	savePEMCert("cert.pem", derBytes)

	keybytes, err := ioutil.ReadFile("key.pem")
	if err != nil {
		log.Fatalf("Failed to read generated private key: %s", err)
	}

	certbytes, err := ioutil.ReadFile("cert.pem")
	if err != nil {
		log.Fatalf("Failed to read generated certificate: %s", err)
	}
	return keybytes, certbytes
}

func TestGetLatestCertificate(t *testing.T) {
	cr := &CertReloader{}

	createCertAndKeyFile()
	defer os.Remove(certFile)
	defer os.Remove(keyFile)

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	assert.Nil(t, err, "error from tls load should be nil")
	cr.cert = &cert

	tlsCert := cr.GetLatestCertificate()
	assert.Equal(t, &cert, tlsCert, "expected certificate is not equal")
}

func TestGetLatestKeyAndCert(t *testing.T) {
	cr := &CertReloader{}
	createCertAndKeyFile()
	defer os.Remove(certFile)
	defer os.Remove(keyFile)

	certPem, err := ioutil.ReadFile("cert.pem")
	if err != nil {
		t.Error(err)
	}
	keyPem, err := ioutil.ReadFile("key.pem")
	if err != nil {
		t.Error(err)
	}
	cr.certPEM = certPem
	cr.keyPEM = keyPem

	key, cert := cr.GetLatestKeyAndCert()
	assert.Equal(t, certPem, cert, "cert should be equal")
	assert.Equal(t, keyPem, key, "key should be equal")
}

func TestMaybeReload(t *testing.T) {
	tests := []struct {
		name         string
		cr           *CertReloader
		preSetup     func(t *testing.T, reloader *CertReloader, origin *Original)
		expectedErr  error
		expectedTime time.Time
		expectedEql  bool
		original     *Original
	}{
		{
			name: "maybeReload should return stat error",
			cr: &CertReloader{
				certFile: certFile,
				mtime:    time.Date(2019, 1, 2, 3, 4, 0, 0, time.UTC),
			},
			preSetup:     func(t *testing.T, cr *CertReloader, origin *Original) {},
			expectedErr:  errors.New("unable to stat cert.pem: stat cert.pem: no such file or directory"),
			expectedTime: time.Date(2019, 1, 2, 3, 4, 0, 0, time.UTC),
			expectedEql:  true,
		},
		{
			name: "maybeReload should not do anything if certificate file has not been modified",
			cr: &CertReloader{
				certFile: certFile,
				keyFile:  keyFile,
			},
			preSetup: func(t *testing.T, cr *CertReloader, origin *Original) {
				// initial setup of key and certs
				var mutex sync.RWMutex
				mutex.Lock()
				keybytes, certbytes := createCertAndKeyFile()
				origin.originalKey = keybytes
				origin.originalCert = certbytes

				// new key and cert
				_, _ = createCertAndKeyFile()
				mutex.Unlock()
				time.Sleep(time.Second * 5)
				cr.mtime = time.Now()
			},
			expectedErr:  nil,
			expectedTime: time.Now().Add(time.Second * 5),
			expectedEql:  true,
		},
		{
			name: "maybeReload should error if key is not found",
			cr: &CertReloader{
				certFile: certFile,
				keyFile:  "nonexistent file",
				mtime:    time.Date(2019, 1, 2, 3, 4, 0, 0, time.UTC),
			},
			preSetup: func(t *testing.T, cr *CertReloader, origin *Original) {
				createCertAndKeyFile()
			},
			expectedErr:  errors.New("unable to load cert from cert.pem,nonexistent file: open nonexistent file: no such file or directory"),
			expectedTime: time.Date(2019, 1, 2, 3, 4, 0, 0, time.UTC),
			expectedEql:  true,
		},
		{
			name: "maybeReload should correctly load the certificate",
			cr: &CertReloader{
				certFile: certFile,
				keyFile:  keyFile,
			},
			preSetup: func(t *testing.T, cr *CertReloader, origin *Original) {
				// initial setup to create mock old key and cert files
				keybytes, certbytes := createCertAndKeyFile()
				// old modified time
				cr.mtime = time.Date(2019, 1, 2, 3, 4, 0, 0, time.UTC)
				origin.originalKey = keybytes
				origin.originalCert = certbytes

				// create new key and cert files and store in cr object
				_, _ = createCertAndKeyFile()
			},
			expectedErr:  nil,
			expectedTime: time.Now(),
			expectedEql:  false,
		},
	}

	for _, test := range tests {
		origin := &Original{}
		test.preSetup(t, test.cr, origin)
		err := test.cr.maybeReload()

		// ignore milliseconds when comparing timestamps
		if test.expectedEql {
			expectedTime := test.expectedTime
			actualTime := test.cr.mtime
			elapsed := actualTime.Sub(expectedTime)
			if elapsed > time.Second {
				t.Error("Modified timestamp is not matched")
			}
		}

		// check whether err is nil and matches expected error
		if err == nil && test.expectedErr == nil {
			assert.Nil(t, err, test.name)

			// check for key PEM contents
			if test.cr.keyPEM != nil && origin.originalKey != nil {
				if test.expectedEql {
					assert.Equal(t, test.cr.keyPEM, origin.originalKey, test.name)
				} else {
					assert.NotEqual(t, test.cr.keyPEM, origin.originalKey, test.name)
				}
			}

			// check for cert PEM contents
			if test.cr.certPEM != nil && origin.originalCert != nil {
				if test.expectedEql {
					assert.Equal(t, test.cr.certPEM, origin.originalCert, test.name)
				} else {
					assert.NotEqual(t, test.cr.certPEM, origin.originalCert, test.name)
				}
			}
		}

		// if error is not nil, check whether the error matches expected error
		if err != nil && test.expectedErr != nil {
			assert.Equal(t, test.expectedErr.Error(), err.Error(), test.name)
		}

		os.Remove(certFile)
		os.Remove(keyFile)
	}
}

func TestTLSReloaderFileUpdate(t *testing.T) {
	log.InitLogger("/tmp/log/test.log", "info")
	cr := &CertReloader{
		certFile: certFile,
		keyFile:  keyFile,
		cond:     abool.New(),
	}

	createCertAndKeyFile()
	defer os.Remove(certFile)
	defer os.Remove(keyFile)
	certPem, err := ioutil.ReadFile("cert.pem")
	if err != nil {
		t.Error(err)
	}
	keyPem, err := ioutil.ReadFile("key.pem")
	if err != nil {
		t.Error(err)
	}

	cr.FileUpdate()
	cr.FileUpdate()
	key, cert := cr.GetLatestKeyAndCert()
	assert.Equal(t, []byte(nil), cert, "cert pem should be nil")
	assert.Equal(t, []byte(nil), key, "key pem should be nil")
	time.Sleep(10 * time.Second)
	key, cert = cr.GetLatestKeyAndCert()
	assert.Equal(t, certPem, cert, "cert pem should be set")
	assert.Equal(t, keyPem, key, "key pem should be set")
}

func TestNewCertReloader(t *testing.T) {
	config := ReloadConfig{
		CertFile: certFile,
		KeyFile:  keyFile,
	}

	createCertAndKeyFile()
	defer os.Remove(certFile)
	defer os.Remove(keyFile)
	certPem, err := ioutil.ReadFile("cert.pem")
	if err != nil {
		t.Error(err)
	}
	keyPem, err := ioutil.ReadFile("key.pem")
	if err != nil {
		t.Error(err)
	}

	stopCh := make(chan struct{})
	newCR, err := NewCertReloader(config, stopCh)
	assert.Nil(t, err, "NewCertReloader should return nil")
	assert.Equal(t, certFile, newCR.certFile, "cert file should be set")
	assert.Equal(t, keyFile, newCR.keyFile, "key file should be set")
	assert.Equal(t, certPem, newCR.certPEM, "cert pem should be set")
	assert.Equal(t, keyPem, newCR.keyPEM, "key pem should be set")
}
