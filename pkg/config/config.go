package config

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/filesystem"
	"k8s.io/klog/v2"
	"sync"
)

type Config interface {
	Get(namespace string, serviceAccount string) *ContainerCredentialsPatchConfig
}

type FileConfig struct {
	containerCredentialsAudience string
	containersCredentialsFullUri string

	watcher              *filesystem.FileWatcher
	identityConfigObject *IdentityConfigObject
	cache                map[Identity]bool
	mu                   sync.RWMutex // guards cache
}

type ContainerCredentialsPatchConfig struct {
	Audience string
	FullUri  string
}

func NewFileConfig(containersCredentialsAudience, containersCredentialsFullUri string) *FileConfig {
	return &FileConfig{
		containerCredentialsAudience: containersCredentialsAudience,
		containersCredentialsFullUri: containersCredentialsFullUri,
		identityConfigObject:         nil,
		cache:                        make(map[Identity]bool),
	}
}

func (f *FileConfig) StartWatcher(ctx context.Context, filePath string) error {
	f.watcher = filesystem.NewFileWatcher("local-file-config", filePath, f.Load)
	return f.watcher.Watch(ctx)
}

func (f *FileConfig) Load(content []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if content == nil || len(content) == 0 {
		klog.Info("Config file is empty, clearing cache")
		f.identityConfigObject = nil
		f.cache = nil
		return nil
	}

	var configObject IdentityConfigObject
	if err := json.Unmarshal(content, &configObject); err != nil {
		return fmt.Errorf("error Unmarshalling config file: %v", err)
	}

	newCache := make(map[Identity]bool)
	for _, item := range configObject.Identities {
		klog.V(5).Infof("Adding SA %s/%s to config cache", item.Namespace, item.ServiceAccount)
		newCache[item] = true
	}
	f.identityConfigObject = &configObject
	f.cache = newCache
	klog.Info("Successfully loaded config file")

	return nil
}

func (f *FileConfig) Get(namespace string, serviceAccount string) *ContainerCredentialsPatchConfig {
	key := Identity{
		Namespace:      namespace,
		ServiceAccount: serviceAccount,
	}
	if f.getCacheItem(key) {
		return &ContainerCredentialsPatchConfig{
			Audience: f.containerCredentialsAudience,
			FullUri:  f.containersCredentialsFullUri,
		}
	}

	return nil
}

func (f *FileConfig) getCacheItem(identity Identity) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.cache[identity]
}
