package cache

import (
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
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
		webhookUsage:     prometheus.NewGauge(prometheus.GaugeOpts{}),
	}

	role, aud, useRegionalSTS, tokenExpiration := cache.Get("default", "default")

	if role != "" || aud != "" {
		t.Errorf("Expected role and aud to be empty, got %s, %s, %t, %d", role, aud, useRegionalSTS, tokenExpiration)
	}

	cache.addSA(testSA)

	role, aud, useRegionalSTS, tokenExpiration = cache.Get("default", "default")

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

			cache := New(audience, "eks.amazonaws.com", tc.defaultRegionalSTS, 86400, informer)
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
				return len(cache.(*serviceAccountCache).cache) != 0, nil
			})
			if err != nil {
				t.Fatalf("cache never called addSA: %v", err)
			}

			gotRoleArn, gotAudience, useRegionalSTS, gotTokenExpiration := cache.Get("default", "default")
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
