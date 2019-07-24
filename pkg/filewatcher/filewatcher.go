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
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/yahoo/k8s-athenz-syncer/pkg/log"
)

// workers type: map[string][]string
// key=directory (/temp/a/), value=files in the directory ([test.txt], which would be saved under /temp/a/test.txt)
type watcher struct {
	fw      WatchNotifier
	workers map[string][]string
	files   []string
}

// WatchNotifier - interface
type WatchNotifier interface {
	FileUpdate()
	WatchError(error)
}

// NewWatcher returns a watcher object with initialized contents.
func NewWatcher(fw WatchNotifier, files []string) *watcher {
	return &watcher{fw, make(map[string][]string), files}
}

// addWorker will extract the directory and base file from the full filepath
// and adds the file as a worker. The directory is the key to access the
// workers.
func (w *watcher) addWorker(file string) string {
	newfile := filepath.Clean(file)
	dir := filepath.Dir(newfile)
	newfile = filepath.Base(newfile)

	if workers, exists := w.workers[dir]; exists {
		w.workers[dir] = append(workers, newfile)
	} else {
		w.workers[dir] = []string{newfile}
	}
	return dir
}

// processWork will iterate through all workers for the file events directory.
func (w *watcher) processWork(event fsnotify.Event) {
	dir := filepath.Dir(event.Name)
	for _, file := range w.workers[dir] {
		if file == filepath.Base(event.Name) {
			log.Infof("File watcher captured new update event. Event: %v", event)
			w.fw.FileUpdate()
		}
	}
}

// Run will create and start a watcher on the directory of the input files.
func (w *watcher) Run(stopCh <-chan struct{}) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	for _, file := range w.files {
		dir := w.addWorker(file)
		err = watcher.Add(dir)
		if err != nil {
			return err
		}
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case event := <-watcher.Events:
				w.processWork(event)
			case err := <-watcher.Errors:
				w.fw.WatchError(err)
				return
			case <-stopCh:
				log.Infoln("File Watcher is stopped.")
				return
			}
		}
	}()

	return nil
}
