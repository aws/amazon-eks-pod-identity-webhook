package cache

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"
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
		saCache:          map[string]*CacheResponse{},
		defaultAudience:  "sts.amazonaws.com",
		annotationPrefix: "eks.amazonaws.com",
		webhookUsage:     prometheus.NewGauge(prometheus.GaugeOpts{}),
	}

	role, aud, useRegionalSTS, tokenExpiration, err := cache.Get("default", "default")
	if err == nil {
		t.Fatal("Expected err to not be empty")
	}
	if role != "" || aud != "" {
		t.Errorf("Expected role and aud to be empty, got %s, %s, %t, %d", role, aud, useRegionalSTS, tokenExpiration)
	}

	cache.addSA(testSA)

	role, aud, useRegionalSTS, tokenExpiration, err = cache.Get("default", "default")

	assert.Equal(t, roleArn, role, "Expected role to be %s, got %s", roleArn, role)
	assert.Equal(t, "sts.amazonaws.com", aud, "Expected aud to be sts.amzonaws.com, got %s", aud)
	assert.True(t, useRegionalSTS, "Expected regional STS to be true, got false")
	assert.Equal(t, int64(3600), tokenExpiration, "Expected token expiration to be 3600, got %d", tokenExpiration)
}

func TestNonRegionalSTS(t *testing.T) {
	trueStr := "true"
	falseStr := "false"
	emptyStr := ""
	roleArn := "arn:aws:iam::111122223333:role/s3-reader"
	audience := "sts.amazonaws.com"
	tokenExpiration := "3600"
	testCases := []struct {
		name                   string
		regionalSTSAnnotation  *string
		defaultRegionalSTS     bool
		expectedUseRegionalSts bool
	}{
		{
			name:                   "annotation true, default false, expect true",
			regionalSTSAnnotation:  &trueStr,
			defaultRegionalSTS:     false,
			expectedUseRegionalSts: true,
		},
		{
			name:                   "annotation false, default false, expect false",
			regionalSTSAnnotation:  &falseStr,
			defaultRegionalSTS:     false,
			expectedUseRegionalSts: false,
		},
		{
			name:                   "annotation empty, default false, expect false",
			regionalSTSAnnotation:  &emptyStr,
			defaultRegionalSTS:     false,
			expectedUseRegionalSts: false,
		},
		{
			name:                   "no annotation, default false, expect false",
			regionalSTSAnnotation:  nil,
			defaultRegionalSTS:     false,
			expectedUseRegionalSts: false,
		},
		{
			name:                   "annotation true, default true, expect true",
			regionalSTSAnnotation:  &trueStr,
			defaultRegionalSTS:     true,
			expectedUseRegionalSts: true,
		},
		{
			name:                   "annotation false, default true, expect false",
			regionalSTSAnnotation:  &falseStr,
			defaultRegionalSTS:     true,
			expectedUseRegionalSts: false,
		},
		{
			name:                   "annotation empty, default true, expect true",
			regionalSTSAnnotation:  &emptyStr,
			defaultRegionalSTS:     true,
			expectedUseRegionalSts: true,
		},
		{
			name:                   "no annotation, default true, expect true",
			regionalSTSAnnotation:  nil,
			defaultRegionalSTS:     true,
			expectedUseRegionalSts: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testSA := &v1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default",
					Namespace: "default",
					Annotations: map[string]string{
						"eks.amazonaws.com/role-arn":         roleArn,
						"eks.amazonaws.com/token-expiration": tokenExpiration,
					},
				},
			}
			if tc.regionalSTSAnnotation != nil {
				testSA.ObjectMeta.Annotations["eks.amazonaws.com/sts-regional-endpoints"] = *tc.regionalSTSAnnotation
			}

			fakeClient := fake.NewSimpleClientset(testSA)
			informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			informer := informerFactory.Core().V1().ServiceAccounts()

			cache := New(audience, "eks.amazonaws.com", tc.defaultRegionalSTS, 86400, informer, nil)
			cache.(*serviceAccountCache).hasSynced = func() bool { return true }
			stop := make(chan struct{})
			informerFactory.Start(stop)
			cache.Start(stop)
			defer close(stop)

			err := wait.ExponentialBackoff(wait.Backoff{Duration: 10 * time.Millisecond, Factor: 1.0, Steps: 3}, func() (bool, error) {
				return len(fakeClient.Actions()) != 0, nil
			})
			if err != nil {
				t.Fatalf("informer never called client: %v", err)
			}

			err = wait.ExponentialBackoff(wait.Backoff{Duration: 10 * time.Millisecond, Factor: 1.0, Steps: 3}, func() (bool, error) {
				return len(cache.(*serviceAccountCache).saCache) != 0, nil
			})
			if err != nil {
				t.Fatalf("cache never called addSA: %v", err)
			}

			gotRoleArn, gotAudience, useRegionalSTS, gotTokenExpiration, err := cache.Get("default", "default")
			if err != nil {
				t.Fatal(err)
			}
			if gotRoleArn != roleArn {
				t.Errorf("got roleArn %v, expected %v", gotRoleArn, roleArn)
			}
			if gotAudience != audience {
				t.Errorf("got audience %v, expected %v", gotAudience, audience)
			}
			if strconv.Itoa(int(gotTokenExpiration)) != tokenExpiration {
				t.Errorf("got token expiration %v, expected %v", gotTokenExpiration, tokenExpiration)
			}
			if useRegionalSTS != tc.expectedUseRegionalSts {
				t.Errorf("got use regional STS %v, expected %v", useRegionalSTS, tc.expectedUseRegionalSts)
			}
		})
	}
}

func TestPopulateCacheFromCM(t *testing.T) {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-identity-webhook",
		},
		Data: map[string]string{
			"config": "{\"myns/mysa\":{\"RoleARN\":\"arn:aws:iam::111122223333:role/s3-reader\"},\"myns2/mysa2\": {\"RoleARN\":\"arn:aws:iam::111122223333:role/s3-reader2\"}}",
		},
	}
	cm2 := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-identity-webhook",
		},
		Data: map[string]string{
			"config": "{\"myns/mysa\":{\"RoleARN\":\"arn:aws:iam::111122223333:role/s3-reader\"}}",
		},
	}

	c := serviceAccountCache{
		cmCache: make(map[string]*CacheResponse),
	}

	{
		err := c.populateCacheFromCM(nil, cm)
		if err != nil {
			t.Errorf("failed to build cache: %v", err)
		}

		role, _, _, _, err := c.Get("mysa2", "myns2")
		if err != nil {
			t.Fatal(err)
		}
		if role == "" {
			t.Errorf("cloud not find entry that should have been added")
		}
	}

	{
		err := c.populateCacheFromCM(cm, cm)
		if err != nil {
			t.Errorf("failed to build cache: %v", err)
		}

		role, _, _, _, err := c.Get("mysa2", "myns2")
		if err != nil {
			t.Fatal(err)
		}
		if role == "" {
			t.Errorf("cloud not find entry that should have been added")
		}
	}

	{
		err := c.populateCacheFromCM(cm, cm2)
		if err != nil {
			t.Errorf("failed to build cache: %v", err)
		}

		role, _, _, _, _ := c.Get("mysa2", "myns2")

		if role != "" {
			t.Errorf("found entry that should have been removed")
		}
	}

}

func TestSAAnnotationRemoval(t *testing.T) {
	roleArn := "arn:aws:iam::111122223333:role/s3-reader"
	oldSA := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
			Annotations: map[string]string{
				"eks.amazonaws.com/role-arn":         roleArn,
				"eks.amazonaws.com/token-expiration": "3600",
			},
		},
	}

	c := serviceAccountCache{
		saCache:          make(map[string]*CacheResponse),
		annotationPrefix: "eks.amazonaws.com",
		webhookUsage:     prometheus.NewGauge(prometheus.GaugeOpts{}),
	}

	c.addSA(oldSA)

	{
		gotRoleArn, _, _, _, err := c.Get("default", "default")
		if err != nil {
			t.Fatal(err)
		}
		if gotRoleArn != roleArn {
			t.Errorf("got roleArn %q, expected %q", gotRoleArn, roleArn)
		}
	}

	newSA := oldSA.DeepCopy()
	newSA.ObjectMeta.Annotations = make(map[string]string)

	c.addSA(newSA)

	{
		gotRoleArn, _, _, _, err := c.Get("default", "default")
		if err != nil {
			t.Fatal(err)
		}
		if gotRoleArn != "" {
			t.Errorf("got roleArn %v, expected %q", gotRoleArn, "")
		}
	}
}

func TestCachePrecedence(t *testing.T) {
	roleArn := "arn:aws:iam::111122223333:role/s3-reader"
	saTokenExpiration := 3600
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-identity-webhook",
		},
		Data: map[string]string{
			"config": "{\"myns/mysa\":{\"RoleARN\":\"arn:aws:iam::111122223333:role/s3-reader\"},\"myns2/mysa2\": {\"RoleARN\":\"arn:aws:iam::111122223333:role/s3-reader2\"}}",
		},
	}
	cm2 := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-identity-webhook",
		},
		Data: map[string]string{
			"config": "{\"myns/mysa\":{\"RoleARN\":\"arn:aws:iam::111122223333:role/s3-reader\"}}",
		},
	}
	sa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mysa2",
			Namespace: "myns2",
			Annotations: map[string]string{
				"eks.amazonaws.com/role-arn":         roleArn,
				"eks.amazonaws.com/token-expiration": fmt.Sprintf("%d", saTokenExpiration),
			},
		},
	}

	sa2 := sa.DeepCopy()
	sa2.ObjectMeta.Annotations = make(map[string]string)

	c := serviceAccountCache{
		saCache:                make(map[string]*CacheResponse),
		cmCache:                make(map[string]*CacheResponse),
		defaultTokenExpiration: pkg.DefaultTokenExpiration,
		annotationPrefix:       "eks.amazonaws.com",
		webhookUsage:           prometheus.NewGauge(prometheus.GaugeOpts{}),
	}

	{
		c.addSA(sa)
		err := c.populateCacheFromCM(nil, cm)
		if err != nil {
			t.Errorf("failed to build cache: %v", err)
		}

		role, _, _, exp, err := c.Get("mysa2", "myns2")
		if err != nil {
			t.Fatal(err)
		}
		if role == "" {
			t.Errorf("could not find entry that should have been added")
		}
		// We expect that the SA still holds presedence
		if exp != int64(saTokenExpiration) {
			t.Errorf("expected tokenExpiration %d, got %d", saTokenExpiration, exp)
		}
	}

	{
		err := c.populateCacheFromCM(cm, cm2)
		if err != nil {
			t.Errorf("failed to build cache: %v", err)
		}

		// Removing sa2 from CM, but SA still exists
		role, _, _, exp, err := c.Get("mysa2", "myns2")
		if err != nil {
			t.Fatal(err)
		}
		if role == "" {
			t.Errorf("could not find entry that should still exist")
		}

		// Note that Get returns default expiration if mapping is not found in the cache.
		if exp != int64(saTokenExpiration) {
			t.Errorf("expected tokenExpiration %d, got %d", saTokenExpiration, exp)
		}
	}

	{
		// Removing annotation
		c.addSA(sa2)

		// Neither cache should return any hits now
		role, _, _, _, err := c.Get("myns2", "mysa2")
		if err == nil {
			t.Errorf("found entry that should not exist")
		}
		if role != "" {
			t.Errorf("found entry that should not exist")
		}

	}
	{
		klog.Info("CM")
		// Adding CM back in. This time only the CM entry exists.
		err := c.populateCacheFromCM(nil, cm)
		if err != nil {
			t.Errorf("failed to build cache: %v", err)
		}

		role, _, _, exp, err := c.Get("mysa2", "myns2")
		if err != nil {
			t.Fatal(err)
		}
		if role == "" {
			t.Errorf("cloud not find entry that should have been added")
		}

		if exp != pkg.DefaultTokenExpiration {
			t.Errorf("expected tokenExpiration %d, got %d", pkg.DefaultTokenExpiration, exp)
		}
	}

}
