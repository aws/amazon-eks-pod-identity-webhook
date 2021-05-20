package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestSaCache(t *testing.T) {
	testSA := &v1.ServiceAccount{}
	testSA.Name = "default"
	testSA.Namespace = "default"
	roleArn := "arn:aws:iam::111122223333:role/s3-reader"
	testSA.Annotations = map[string]string{
		"eks.amazonaws.com/role-arn":               roleArn,
		"eks.amazonaws.com/sts-regional-endpoints": "true",
		"eks.amazonaws.com/token-expiration":       "3600",
	}

	cache := &serviceAccountCache{
		cache:            map[string]*CacheResponse{},
		defaultAudience:  "sts.amazonaws.com",
		annotationPrefix: "eks.amazonaws.com",
	}

	role, aud, useRegionalSTS, tokenExpiration := cache.Get("default", "default")

	if role != "" || aud != "" {
		t.Errorf("Expected role and aud to be empty, got %s, %s, %t, %d", role, aud, useRegionalSTS, tokenExpiration)
	}

	cache.addSA(testSA)

	role, aud, useRegionalSTS, tokenExpiration = cache.Get("default", "default")

	assert.Equal(t, roleArn, role, "Expected role to be %s, got %s", roleArn, role)
	assert.Equal(t, "sts.amazonaws.com", aud, "Expected aud to be sts.amzonaws.com, got %s", aud)
	assert.Equal(t, true, useRegionalSTS, "Expected regional STS to be true, got false")
	assert.Equal(t, int64(3600), tokenExpiration, "Expected token expiration to be 3600, got %d", tokenExpiration)
}

func TestNonRegionalSTS(t *testing.T) {
	testSA := &v1.ServiceAccount{}
	testSA.Name = "default"
	testSA.Namespace = "default"
	roleArn := "arn:aws:iam::111122223333:role/s3-reader"
	testSA.Annotations = map[string]string{
		"eks.amazonaws.com/role-arn":               roleArn,
		"eks.amazonaws.com/sts-regional-endpoints": "false",
		"eks.amazonaws.com/token-expiration":       "3600",
	}

	cache := &serviceAccountCache{
		cache:            map[string]*CacheResponse{},
		defaultAudience:  "sts.amazonaws.com",
		annotationPrefix: "eks.amazonaws.com",
	}

	cache.addSA(testSA)

	_, _, useRegionalSTS, _ := cache.Get("default", "default")

	assert.Equal(t, false, useRegionalSTS, "Expected regional STS to be true, got false")
}
