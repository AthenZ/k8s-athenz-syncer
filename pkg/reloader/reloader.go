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
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/AthenZ/k8s-athenz-syncer/pkg/filewatcher"
	"github.com/AthenZ/k8s-athenz-syncer/pkg/log"
	"github.com/pkg/errors"
	"github.com/tevino/abool"
)

// CertReloader reloads the (key, cert) pair from the filesystem when
// the cert file is updated.
type CertReloader struct {
	l        sync.RWMutex
	certFile string
	keyFile  string
	cert     *tls.Certificate
	certPEM  []byte
	keyPEM   []byte
	mtime    time.Time
	cond     *abool.AtomicBool
}

// ReloadConfig contains the config for cert reload.
type ReloadConfig struct {
	CertFile string // the cert file
	KeyFile  string // the key file
}

// GetLatestCertificate returns the latest known certificate.
func (w *CertReloader) GetLatestCertificate() *tls.Certificate {
	w.l.RLock()
	c := w.cert
	w.l.RUnlock()
	return c
}

// GetLatestKeyAndCert returns the latest known key and certificate in raw bytes.
func (w *CertReloader) GetLatestKeyAndCert() ([]byte, []byte) {
	w.l.RLock()
	k := w.keyPEM
	c := w.certPEM
	w.l.RUnlock()
	return k, c
}

// fileUpdate calls maybeReload to reload the certificate from the filesystem. This
// function is called by multiple go routines, only the first one will call the
// function after a 5 second sleep in order to group the file watch events together.
func (w *CertReloader) fileUpdate() {
	if w.cond.SetToIf(false, true) {
		time.Sleep(time.Second * 5)
		err := w.maybeReload()
		if err != nil {
			log.Errorln("Error reloading certificate:", err)
		}
		w.cond.SetToIf(true, false)
	}
}

// FileUpdate spawns a internal fileUpdate go routine.
func (w *CertReloader) FileUpdate() {
	go w.fileUpdate()
}

// WatchError will process any errors received from the file watch.
func (w *CertReloader) WatchError(err error) {
	log.Errorln("Error watching cert and key files:", err)
}

// maybeReload reloads the certificate if the filesystem contents has changed
func (w *CertReloader) maybeReload() error {
	st, err := os.Stat(w.certFile)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("unable to stat %s", w.certFile))
	}
	w.l.RLock()
	mtime := w.mtime
	w.l.RUnlock()
	if !st.ModTime().After(mtime) {
		return nil
	}
	cert, certPEM, keyPEM, err := w.reloadKeyCert()
	if err != nil {
		return err
	}

	w.l.Lock()
	w.cert = &cert
	w.certPEM = certPEM
	w.keyPEM = keyPEM
	w.mtime = st.ModTime()
	w.l.Unlock()
	return nil
}

func (w *CertReloader) reloadKeyCert() (tls.Certificate, []byte, []byte, error) {
	cert, err := tls.LoadX509KeyPair(w.certFile, w.keyFile)
	if err != nil {
		return tls.Certificate{}, nil, nil, errors.Wrap(err, fmt.Sprintf("unable to load cert from %s,%s", w.certFile, w.keyFile))
	}
	certPEM, err := ioutil.ReadFile(w.certFile)
	if err != nil {
		return tls.Certificate{}, nil, nil, errors.Wrap(err, fmt.Sprintf("unable to load cert from %s", w.certFile))
	}
	keyPEM, err := ioutil.ReadFile(w.keyFile)
	if err != nil {
		return tls.Certificate{}, nil, nil, errors.Wrap(err, fmt.Sprintf("unable to load key from %s", w.keyFile))
	}
	return cert, certPEM, keyPEM, nil
}

// NewCertReloader returns a CertReloader that starts a file watcher which reloads
// the (key, cert) pair whenever the cert file changes on the filesystem.
func NewCertReloader(config ReloadConfig, stopCh <-chan struct{}) (*CertReloader, error) {
	r := &CertReloader{
		certFile: config.CertFile,
		keyFile:  config.KeyFile,
		cond:     abool.New(),
	}
	files := []string{r.certFile, r.keyFile}
	watcher := filewatcher.NewWatcher(r, files)
	err := watcher.Run(stopCh)
	if err != nil {
		return nil, err
	}

	return r, r.maybeReload()
}