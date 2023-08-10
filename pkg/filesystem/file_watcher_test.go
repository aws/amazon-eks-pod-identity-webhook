package filesystem

import (
	"context"
	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fileContentRecorder struct {
	content string
}

func (f *fileContentRecorder) record(content []byte) error {
	f.content = string(content)
	return nil
}

const defaultTimeout = 10 * time.Second
const defaultPollInterval = 1 * time.Second

func TestFileWatcher(t *testing.T) {
	noopFileFunc := func(t *testing.T, fileName string) {}

	testcases := []struct {
		name              string
		preWatchFileFunc  func(t *testing.T, filePath string)
		postWatchFileFunc func(t *testing.T, filePath string)
		expectedContent   string
	}{
		{
			name:              "Missing file",
			preWatchFileFunc:  noopFileFunc,
			postWatchFileFunc: noopFileFunc,
			expectedContent:   "",
		},
		{
			name: "Empty file",
			preWatchFileFunc: func(t *testing.T, filePath string) {
				_, err := os.Create(filePath)
				assert.NoError(t, err)
			},
			postWatchFileFunc: noopFileFunc,
			expectedContent:   "",
		},
		{
			name: "File exists before watch is started",
			preWatchFileFunc: func(t *testing.T, filePath string) {
				file, err := os.Create(filePath)
				assert.NoError(t, err)
				setFileContent(t, file, "foo")
			},
			postWatchFileFunc: noopFileFunc,
			expectedContent:   "foo",
		},
		{
			name:             "File is created after watch",
			preWatchFileFunc: noopFileFunc,
			postWatchFileFunc: func(t *testing.T, filePath string) {
				file, err := os.Create(filePath)
				assert.NoError(t, err)
				setFileContent(t, file, "bar")
			},
			expectedContent: "bar",
		},
		{
			name: "File is removed after watch",
			preWatchFileFunc: func(t *testing.T, filePath string) {
				file, err := os.Create(filePath)
				assert.NoError(t, err)
				setFileContent(t, file, "bar")
			},
			postWatchFileFunc: func(t *testing.T, filePath string) {
				err := os.Remove(filePath)
				assert.NoError(t, err)
			},
			expectedContent: "",
		},
		{
			name: "Basic test",
			preWatchFileFunc: func(t *testing.T, filePath string) {
				file, err := os.Create(filePath)
				assert.NoError(t, err)
				setFileContent(t, file, "foo")
			},
			postWatchFileFunc: func(t *testing.T, filePath string) {
				file, err := os.OpenFile(filePath, os.O_RDWR|os.O_APPEND, 0666)
				assert.NoError(t, err)
				appendFileContent(t, file, "-bar")
				appendFileContent(t, file, "-end")
			},
			expectedContent: "foo-bar-end",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			dirPath, err := os.MkdirTemp("", "test")
			assert.NoError(t, err)
			defer os.RemoveAll(dirPath)

			filePath := filepath.Join(dirPath, "file")
			tc.preWatchFileFunc(t, filePath)

			recorder := fileContentRecorder{}
			fileWatcher := NewFileWatcher("testing", filePath, recorder.record)
			err = fileWatcher.Watch(ctx)
			assert.NoError(t, err)
			assert.NotEmpty(t, fileWatcher.watcher.WatchList())

			tc.postWatchFileFunc(t, filePath)
			assert.Eventually(t, func() bool {
				return tc.expectedContent == recorder.content
			}, defaultTimeout, defaultPollInterval)

			klog.InfoS("WatchList len", len(fileWatcher.watcher.WatchList()))
			cancel()
			assert.Eventuallyf(t, func() bool {
				return len(fileWatcher.watcher.WatchList()) == 0
			}, defaultTimeout, defaultPollInterval, "expect watchList to have len 0, but actual len is %d", len(fileWatcher.watcher.WatchList()))
		})
	}
}

func setFileContent(t *testing.T, f *os.File, newContent string) {
	err := f.Truncate(0)
	assert.NoError(t, err)
	_, err = f.Seek(0, 0)
	assert.NoError(t, err)
	_, err = f.WriteString(newContent)
	assert.NoError(t, err)
	err = f.Sync()
	assert.NoError(t, err)
}

func appendFileContent(t *testing.T, f *os.File, newContent string) {
	_, err := f.WriteString(newContent)
	assert.NoError(t, err)
	err = f.Sync()
	assert.NoError(t, err)
}
