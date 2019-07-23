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
package filewatcher

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/yahoo/k8s-athenz-syncer/pkg/log"
	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
)

type fakeReloader struct {
	l           sync.RWMutex
	fnCallCount int
}

func (f *fakeReloader) FileUpdate() {
	f.l.Lock()
	f.fnCallCount++
	f.l.Unlock()
}

func (f *fakeReloader) WatchError(err error) {}

func TestNewWatcher(t *testing.T) {
	fr := &fakeReloader{}
	files := []string{}
	watcher := NewWatcher(fr, files)
	assert.Equal(t, fr, watcher.fw, "watcher objects are not equal")
}

func TestAddWorker(t *testing.T) {
	files := []string{"/tmp/file1", "/tmp/file2"}
	watcher := NewWatcher(&fakeReloader{}, files)
	watcher.addWorker("/tmp/file1")
	watcher.addWorker("/tmp/file2")
	assert.Equal(t, "file1", watcher.workers["/tmp"][0], "file1 worker not found in /tmp dir")
	assert.Equal(t, "file2", watcher.workers["/tmp"][1], "file2 worker not found in /tmp dir")
}

func TestProcessWork(t *testing.T) {
	log.InitLogger("/tmp/log/test.log", "info")
	fr := &fakeReloader{}
	files := []string{"/tmp/file1", "/tmp/file2"}
	watcher := NewWatcher(fr, files)
	watcher.workers["/tmp"] = []string{"file1", "file2"}
	watcher.processWork(fsnotify.Event{
		Name: "/tmp/file1",
		Op:   fsnotify.Create,
	})
	watcher.processWork(fsnotify.Event{
		Name: "/tmp/file2",
		Op:   fsnotify.Create,
	})
	assert.Equal(t, 2, fr.fnCallCount, "function FileUpdate should have been called twice")
}

func TestRun(t *testing.T) {
	log.InitLogger("/tmp/log/test.log", "info")
	fr := &fakeReloader{}
	dir, err := os.Getwd()
	assert.Nil(t, err, "get working directory error should be nil")

	files := []string{dir + "/file1"}
	watcher := NewWatcher(fr, files)

	stopCh := make(chan struct{})
	err = watcher.Run(stopCh)
	assert.Nil(t, err, "watcher run error should be nil")

	f, err := os.Create(dir + "/file1")
	defer os.Remove(f.Name())
	assert.Nil(t, err, "os create error should be nil")

	time.Sleep(1 * time.Second)
	fr.l.RLock()
	assert.Equal(t, 1, fr.fnCallCount, "function FileUpdate should have been called once")
	fr.l.RUnlock()
}
