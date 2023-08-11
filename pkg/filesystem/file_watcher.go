/*
  Copyright 2023 Amazon.com, Inc. or its affiliates. All Rights Reserved.

  Licensed under the Apache License, Version 2.0 (the "License").
  You may not use this file except in compliance with the License.
  A copy of the License is located at

      http://www.apache.org/licenses/LICENSE-2.0

  or in the "license" file accompanying this file. This file is distributed
  on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
  express or implied. See the License for the specific language governing
  permissions and limitations under the License.
*/

package filesystem

import (
	"context"
	"errors"
	"github.com/fsnotify/fsnotify"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
	"time"
)

const (
	workItemKey        = "key"
	workerPollInterval = 1 * time.Second
	workqueueBaseDelay = 10 * time.Millisecond
	workqueueMaxDelay  = 5 * time.Minute
)

// FileWatcher watches a single file and trigger the given handler function
type FileWatcher struct {
	path    string
	handler FileContentHandler

	watcher *fsnotify.Watcher

	// Instead of doing the work in processEvent, a queue is used primarily to
	// make testing easier and to keep the FileWatcher simple.  A single item
	// will be added to the queue to denote the file should be reloaded.
	// Additional events will be deduped until the item is removed with Done().
	// If there is an error reloading the file, we enqueue rate limited (with a
	// max wait of 10 seconds).  The workqueue was chosen because it allows us
	// to deduplicate reloads and retry with rate limit on failure.  This
	// pattern is borrowed from
	// https://github.com/kubernetes/kubernetes/blob/3d67e162a03d0d724dc5a15a0617c5e8572c7b4a/staging/src/k8s.io/apiserver/pkg/server/dynamiccertificates/dynamic_serving_content.go
	queue workqueue.RateLimitingInterface
}

type FileContentHandler func(content []byte) error

// NewFileWatcher creates a FileWatcher
func NewFileWatcher(purpose string, path string, handler FileContentHandler) *FileWatcher {
	return &FileWatcher{
		path:    path,
		handler: handler,
		queue:   workqueue.NewNamedRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(workqueueBaseDelay, workqueueMaxDelay), purpose),
	}
}

// Watch sets up the fsnotify watcher and add the file that we are interested in.  The file watcher
// and worker run in goroutines.  The goroutines are stopped when the ctx is cancelled.
func (f *FileWatcher) Watch(ctx context.Context) error {
	// Trigger initial file load
	f.queue.Add(workItemKey)

	var err error
	f.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	go wait.UntilWithContext(ctx, f.runWorker, workerPollInterval)

	// Start listening for events.
	go func() {
		for {
			select {
			case err := <-f.watcher.Errors:
				klog.ErrorS(err, "Error from watcher")
			case e := <-f.watcher.Events:
				klog.V(3).InfoS("Event received", "event", e)
				f.processEvent(e)
			case <-ctx.Done():
				klog.Info("context closed, stopping FileWatcher")
				f.watcher.Close()
				return
			}
		}
	}()

	dir := filepath.Dir(f.path)
	err = f.watcher.Add(dir)
	if err != nil {
		klog.Fatal(err)
	}

	return nil
}

// processEvent adds an item to the workqueue.
func (f *FileWatcher) processEvent(event fsnotify.Event) {
	if event.Name == f.path {
		f.queue.Add(workItemKey)
	}
}

func (f *FileWatcher) runWorker(ctx context.Context) {
	for f.processNextWorkItem(ctx) {
	}
}

func (f *FileWatcher) processNextWorkItem(ctx context.Context) (continuePoll bool) {
	k, quit := f.queue.Get()
	if quit {
		return false
	}
	defer f.queue.Done(k)

	if err := f.loadFile(); err != nil {
		klog.ErrorS(err, "failed processing files")
		f.queue.AddRateLimited(k)
		return true
	}

	f.queue.Forget(k)
	return true
}

func (f *FileWatcher) loadFile() error {
	if _, err := os.Stat(f.path); errors.Is(err, os.ErrNotExist) {
		return f.handler(nil)
	}

	content, err := os.ReadFile(f.path)
	if err != nil {
		return err
	}
	return f.handler(content)
}
