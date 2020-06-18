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

import(
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/yahoo/k8s-athenz-identity/pkg/util"

)

type base struct {
	files *versionedFiles
}

// PrivateKeySource returns signing keys using the latest Version found in a directory.
type PrivateKeySource struct {
	secretName string
	base
}

// SigningKey encapsulates a signing key
type SigningKey struct {
	URI     string        // the URI that identifies the key
	Type    util.KeyType       // key type
	Value   crypto.Signer // the private key
	Version string        // key Version
}

// versionedFiles for private key
type versionedFiles struct {
	dir    string
	prefix string
}

type fv struct {
	file       string
	version    int
	versionStr string
}

// list returns a list of file version in sorted order of version (highest first).
func (f *versionedFiles) list() ([]fv, error) {
	var ret []fv
	prefix := f.prefix + ".v"
	if files, err := ioutil.ReadDir(f.dir); err == nil {
		for _, file := range files {
			name := file.Name()
			if !strings.HasPrefix(name, prefix) {
				if !strings.HasPrefix(name, ".") {
					log.Printf("invalid versioned file '%s', does not start with '%s'", name, prefix)
				}
				continue
			}
			v := strings.TrimPrefix(name, prefix)
			if version, err := strconv.Atoi(v); err == nil {
				ret = append(ret, fv{
					file:       filepath.Join(f.dir, name),
					version:    version,
					versionStr: fmt.Sprintf("v%d", version),
				})
			} else {
				log.Printf("invalid Version '%s' for file '%s'\n", v, name)
			}
		}
	} else {
		return nil, errors.Wrap(err, "list files")
	}
	if len(ret) == 0 {
		return nil, fmt.Errorf("no versioned files under %s", f.dir)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[j].version < ret[i].version // note: descending sort
	})
	return ret, nil
}

// NewPrivateKeySource returns a private key source that uses files in the supplied directory
// having the supplied prefix. Files in the directory must be named <secret-name>.v<n> to
// be considered. Sorting is not lexicographic; "v10" sorts higher than "v9"
func NewPrivateKeySource(dir string, secretName string) *PrivateKeySource {
	return &PrivateKeySource{
		secretName: secretName,
		base: base{
			files: &versionedFiles{
				dir:    dir,
				prefix: secretName,
			},
		},
	}
}

// SigningKey returns the current signing key.
func (pks *PrivateKeySource) SigningKey() (*SigningKey, error) {
	contents, version, err := pks.files.latestVersionContents()
	if err != nil {
		return nil, err
	}
	keyType, key, err := util.PrivateKeyFromPEMBytes(contents)
	if err != nil {
		return nil, err
	}
	return &SigningKey{
		URI:     secretURI(pks.secretName, version),
		Type:    keyType,
		Value:   key,
		Version: version,
	}, nil
}

// latestVersionContents gets the latested version key contents
func (f *versionedFiles) latestVersionContents() ([]byte, string, error) {
	fvs, err := f.list()
	if err != nil {
		return nil, "", err
	}
	fv := fvs[0]
	content, err := ioutil.ReadFile(fv.file)
	if err != nil {
		return nil, "", err
	}
	return content, fv.versionStr, nil
}

// secretURI gets secret URI
func secretURI(name, version string) string {
	return fmt.Sprintf("secret:%s?Version=%s", name, version)
}

// PEMBytes marshals the signing key to PEM format.
func (s *SigningKey) PEMBytes() ([]byte, error) {
	if privKey, ok := s.Value.(*rsa.PrivateKey); ok {
		return pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privKey),
		}), nil
	}
	if ecdsaKey, ok := s.Value.(*ecdsa.PrivateKey); ok {
		b, err := x509.MarshalECPrivateKey(ecdsaKey)
		if err != nil {
			return nil, errors.Wrap(err, "marshal ECDSA key")
		}
		return pem.EncodeToMemory(&pem.Block{
			Type:  "ECDSA PRIVATE KEY",
			Bytes: b,
		}), nil
	}
	return nil, fmt.Errorf("unexpected error, key was not of RSA or ECDSA type")
}