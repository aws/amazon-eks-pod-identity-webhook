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

const workItemKey = "key"

// FileWatcher watches the files defined in files
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
		queue:   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), purpose),
	}
}

// Watch sets up the fsnotify watcher and adds each file we are interested in.  The file watcher
// and worker run in goroutines.  The function returns and goroutines are stopped when the ctx
// is cancelled.
func (f *FileWatcher) Watch(ctx context.Context) error {
	// Trigger initial file load
	f.queue.Add(workItemKey)

	var err error
	f.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	go wait.UntilWithContext(ctx, f.runWorker, time.Second)

	// Start listening for events.
	go func() {
		for {
			select {
			case err := <-f.watcher.Errors:
				klog.ErrorS(err, "Error from watcher")
			case e := <-f.watcher.Events:
				klog.InfoS("Event received", "event", e)
				f.processEvent(e)
			case <-ctx.Done():
				klog.Info("context closed, stopping FileWatcher")
				f.watcher.Close()
				return
			}
		}
	}()

	// Add a path
	dir := filepath.Dir(f.path)
	err = f.watcher.Add(dir)
	if err != nil {
		klog.Fatal(err)
	}

	return nil
}

// processEvent adds an item to the workqueue so that file reloading work will
// be scheduled to be done.  Rename and Remove events attempt to trigger an
// immediate restart of the watch by removing and re-adding it.
func (f *FileWatcher) processEvent(event fsnotify.Event) {
	if event.Name == f.path {
		f.queue.Add(workItemKey)
	}
}

func (f *FileWatcher) runWorker(ctx context.Context) {
	for f.processNextWorkItem(ctx) {
	}
}

func (f *FileWatcher) processNextWorkItem(ctx context.Context) bool {
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
