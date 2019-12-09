package cache

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestSaCache(t *testing.T) {
	testSA := &v1.ServiceAccount{}
	testSA.Name = "default"
	testSA.Namespace = "default"

	cache := &serviceAccountCache{
		cache:           map[string]*CacheResponse{},
		defaultAudience: "sts.amazonaws.com",
	}

	aud := cache.Get("default", "default")

	if aud != "" {
		t.Errorf("Expected role and aud to be empty, got %s", aud)
	}

	cache.addSA(testSA)

	aud = cache.Get("default", "default")
	if aud != "sts.amazonaws.com" {
		t.Errorf("Expected aud to be sts.amzonaws.com, got %s", aud)
	}

}
