package cache

import (
	"sync"

	v1 "k8s.io/api/core/v1"
)

// FakeServiceAccountCache is a goroutine safe cache for testing
type FakeServiceAccountCache struct {
	mu    sync.RWMutex // guards cache
	cache map[string]*CacheResponse
}

func NewFakeServiceAccountCache(accounts ...*v1.ServiceAccount) *FakeServiceAccountCache {
	c := &FakeServiceAccountCache{
		cache: map[string]*CacheResponse{},
	}
	for _, sa := range accounts {
		audience, ok := sa.Annotations["eks.amazonaws.com/audience"]
		if !ok {
			audience = "sts.amazonaws.com"
		}

		c.Add(sa.Name, sa.Namespace, audience)
	}
	return c
}

var _ ServiceAccountCache = &FakeServiceAccountCache{}

// Start does nothing
func (f *FakeServiceAccountCache) Start() {}

// Get gets a service account from the cache
func (f *FakeServiceAccountCache) Get(name, namespace string) (aud string) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	resp, ok := f.cache[namespace+"/"+name]
	if !ok {
		return ""
	}
	return resp.Audience
}

// Add adds a cache entry
func (f *FakeServiceAccountCache) Add(name, namespace, aud string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cache[namespace+"/"+name] = &CacheResponse{
		Audience: aud,
	}
}

// Pop deletes a cache entry
func (f *FakeServiceAccountCache) Pop(name, namespace string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.cache, namespace+"/"+name)
}
