package cache

import (
	"strconv"
	"sync"

	"k8s.io/api/core/v1"
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
		arn, _ := sa.Annotations["eks.amazonaws.com/role-arn"]
		audience, ok := sa.Annotations["eks.amazonaws.com/audience"]
		if !ok {
			audience = "sts.amazonaws.com"
		}
		var fsGroup *int64
		if fsgStr, ok := sa.Annotations["eks.amazonaws.com/fs-group"]; ok {
			if fsgInt, err := strconv.ParseInt(fsgStr, 10, 64); err == nil {
				fsGroup = &fsgInt
			}
		}

		c.Add(sa.Name, sa.Namespace, arn, audience, fsGroup)
	}
	return c
}

var _ ServiceAccountCache = &FakeServiceAccountCache{}

// Start does nothing
func (f *FakeServiceAccountCache) Start() {}

// Get gets a service account from the cache
func (f *FakeServiceAccountCache) Get(name, namespace string) (role, aud string, fsGroup *int64) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	resp, ok := f.cache[namespace+"/"+name]
	if !ok {
		return "", "", nil
	}
	// Immutable safety for the fsGroup *int64 in the cache
	if resp.FSGroup != nil {
		fsGroup = new(int64)
		*fsGroup = *resp.FSGroup
	}
	return resp.RoleARN, resp.Audience, fsGroup
}

// Add adds a cache entry
func (f *FakeServiceAccountCache) Add(name, namespace, role, aud string, fsGroup *int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cache[namespace+"/"+name] = &CacheResponse{
		RoleARN:  role,
		Audience: aud,
		FSGroup:  fsGroup,
	}
}

// Pop deletes a cache entry
func (f *FakeServiceAccountCache) Pop(name, namespace string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.cache, namespace+"/"+name)
}
