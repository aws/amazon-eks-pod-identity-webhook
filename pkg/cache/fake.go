package cache

import (
	"encoding/json"
	v1 "k8s.io/api/core/v1"
	"strconv"
	"sync"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
)

// FakeServiceAccountCache is a goroutine safe cache for testing
type FakeServiceAccountCache struct {
	mu    sync.RWMutex // guards cache
	cache map[string]*Entry
}

func NewFakeServiceAccountCache(accounts ...*v1.ServiceAccount) *FakeServiceAccountCache {
	c := &FakeServiceAccountCache{
		cache: map[string]*Entry{},
	}
	for _, sa := range accounts {
		arn, _ := sa.Annotations["eks.amazonaws.com/role-arn"]
		audience, ok := sa.Annotations["eks.amazonaws.com/audience"]
		if !ok {
			audience = "sts.amazonaws.com"
		}
		regionalSTSstr, _ := sa.Annotations["eks.amazonaws.com/sts-regional-endpoints"]
		regionalSTS, _ := strconv.ParseBool(regionalSTSstr)
		tokenExpirationStr, _ := sa.Annotations["eks.amazonaws.com/token-expiration"]
		tokenExpiration, err := strconv.ParseInt(tokenExpirationStr, 10, 64)
		if err != nil {
			tokenExpiration = pkg.DefaultTokenExpiration // Otherwise default would be 0
		}

		c.Add(sa.Name, sa.Namespace, arn, audience, regionalSTS, tokenExpiration)
	}
	return c
}

var _ ServiceAccountCache = &FakeServiceAccountCache{}

// Start does nothing
func (f *FakeServiceAccountCache) Start(chan struct{}) {}

// Get gets a service account from the cache
func (f *FakeServiceAccountCache) Get(req Request) Response {
	f.mu.RLock()
	defer f.mu.RUnlock()
	resp, ok := f.cache[req.CacheKey()]
	if !ok {
		return Response{TokenExpiration: pkg.DefaultTokenExpiration}
	}
	return Response{
		RoleARN:         resp.RoleARN,
		Audience:        resp.Audience,
		UseRegionalSTS:  resp.UseRegionalSTS,
		TokenExpiration: resp.TokenExpiration,
		FoundInCache:    true,
	}
}

func (f *FakeServiceAccountCache) GetCommonConfigurations(name, namespace string) (useRegionalSTS bool, tokenExpiration int64) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	resp, ok := f.cache[namespace+"/"+name]
	if !ok {
		return false, pkg.DefaultTokenExpiration
	}
	return resp.UseRegionalSTS, resp.TokenExpiration
}

// Add adds a cache entry
func (f *FakeServiceAccountCache) Add(name, namespace, role, aud string, regionalSTS bool, tokenExpiration int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cache[namespace+"/"+name] = &Entry{
		RoleARN:         role,
		Audience:        aud,
		UseRegionalSTS:  regionalSTS,
		TokenExpiration: tokenExpiration,
	}
}

// Pop deletes a cache entry
func (f *FakeServiceAccountCache) Pop(name, namespace string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.cache, namespace+"/"+name)
}

func (f *FakeServiceAccountCache) ToJSON() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	contents, err := json.MarshalIndent(f.cache, "", " ")
	if err != nil {
		return ""
	}
	return string(contents)
}

func (f *FakeServiceAccountCache) Clear() {
	f.cache = map[string]*Entry{}
}
