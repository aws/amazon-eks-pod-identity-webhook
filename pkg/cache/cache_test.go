package cache

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	awsarn "github.com/aws/aws-sdk-go/aws/arn"
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

	role, aud, useRegionalSTS, tokenExpiration, found := cache.Get("default", "default")

	assert.False(t, found, "Expected no cache entry to be found")
	if role != "" || aud != "" {
		t.Errorf("Expected role and aud to be empty, got %s, %s, %t, %d", role, aud, useRegionalSTS, tokenExpiration)
	}

	cache.addSA(testSA)

	role, aud, useRegionalSTS, tokenExpiration, found = cache.Get("default", "default")

	assert.True(t, found, "Expected cache entry to be found")
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

			testComposeRoleArn := ComposeRoleArn{}

			cache := New(audience, "eks.amazonaws.com", tc.defaultRegionalSTS, 86400, informer, nil, testComposeRoleArn)
			stop := make(chan struct{})
			informerFactory.Start(stop)
			informerFactory.WaitForCacheSync(stop)
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

			gotRoleArn, gotAudience, useRegionalSTS, gotTokenExpiration, found := cache.Get("default", "default")
			assert.True(t, found, "Expected cache entry to be found")
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

		role, _, _, _, _ := c.Get("mysa2", "myns2")
		if role == "" {
			t.Errorf("cloud not find entry that should have been added")
		}
	}

	{
		err := c.populateCacheFromCM(cm, cm)
		if err != nil {
			t.Errorf("failed to build cache: %v", err)
		}

		role, _, _, _, _ := c.Get("mysa2", "myns2")
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
		gotRoleArn, _, _, _, _ := c.Get("default", "default")
		if gotRoleArn != roleArn {
			t.Errorf("got roleArn %q, expected %q", gotRoleArn, roleArn)
		}
	}

	newSA := oldSA.DeepCopy()
	newSA.ObjectMeta.Annotations = make(map[string]string)

	c.addSA(newSA)

	{
		gotRoleArn, _, _, _, _ := c.Get("default", "default")
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

		role, _, _, exp, _ := c.Get("mysa2", "myns2")
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
		role, _, _, exp, _ := c.Get("mysa2", "myns2")
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
		role, _, _, _, _ := c.Get("myns2", "mysa2")
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

		role, _, _, exp, _ := c.Get("mysa2", "myns2")
		if role == "" {
			t.Errorf("cloud not find entry that should have been added")
		}

		if exp != pkg.DefaultTokenExpiration {
			t.Errorf("expected tokenExpiration %d, got %d", pkg.DefaultTokenExpiration, exp)
		}
	}

}

func TestRoleArnComposition(t *testing.T) {
	role := "s3-reader"
	audience := "sts.amazonaws.com"
	tokenExpiration := "3600"
	accountID := "111122223333"
	resource := fmt.Sprintf("role/%s", role)

	testSA := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
			Annotations: map[string]string{
				"eks.amazonaws.com/role-arn":         role,
				"eks.amazonaws.com/token-expiration": tokenExpiration,
			},
		},
	}

	testComposeRoleArn := ComposeRoleArn{
		Enabled: true,

		AccountID: "111122223333",
		Partition: "aws",
		Region:    "us-west-2",
	}

	fakeClient := fake.NewSimpleClientset(testSA)
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	informer := informerFactory.Core().V1().ServiceAccounts()

	cache := New(audience, "eks.amazonaws.com", true, 86400, informer, nil, testComposeRoleArn)
	stop := make(chan struct{})
	informerFactory.Start(stop)
	informerFactory.WaitForCacheSync(stop)

	cache.Start(stop)
	defer close(stop)

	var roleArn string
	err := wait.ExponentialBackoff(wait.Backoff{Duration: 10 * time.Millisecond, Factor: 1.0, Steps: 3}, func() (bool, error) {
		roleArn, _, _, _, _ = cache.Get("default", "default")
		return roleArn != "", nil
	})
	if err != nil {
		t.Fatalf("cache never returned role arn %v", err)
	}

	arn, err := awsarn.Parse(roleArn)

	assert.Nil(t, err, "Expected ARN parsing to succeed")
	assert.True(t, awsarn.IsARN(roleArn), "Expected ARN validation to be true, got false")
	assert.Equal(t, accountID, arn.AccountID, "Expected account ID to be %s, got %s", accountID, arn.AccountID)
	assert.Equal(t, resource, arn.Resource, "Expected resource to be %s, got %s", resource, arn.Resource)
}

func TestGetCommonConfigurations(t *testing.T) {
	const (
		namespaceName      = "foo"
		serviceAccountName = "foo-sa"
	)

	k8sServiceAccount := &v1.ServiceAccount{}
	k8sServiceAccount.Name = serviceAccountName
	k8sServiceAccount.Namespace = namespaceName
	k8sServiceAccount.Annotations = map[string]string{
		"eks.amazonaws.com/sts-regional-endpoints": "true",
		"eks.amazonaws.com/token-expiration":       "10000",
	}

	k8sConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-identity-webhook",
		},
		Data: map[string]string{
			"config": "{\"foo/foo-sa\":{\"RoleARN\":\"arn:aws-test:iam::123456789012:role/my-role\",\"Audience\":\"amazonaws.com\",\"UseRegionalSTS\":true,\"TokenExpiration\":20000}}",
		},
	}

	testcases := []struct {
		name                    string
		serviceAccount          *v1.ServiceAccount
		configMap               *v1.ConfigMap
		requestServiceAccount   string
		requestNamespace        string
		expectedUseRegionalSTS  bool
		expectedTokenExpiration int64
	}{
		{
			name:                    "Entry not found in sa or cm",
			requestServiceAccount:   "sa",
			requestNamespace:        "ns",
			expectedUseRegionalSTS:  false,
			expectedTokenExpiration: pkg.DefaultTokenExpiration,
		},
		{
			name:                    "Service account is set, but not CM",
			serviceAccount:          k8sServiceAccount,
			requestServiceAccount:   serviceAccountName,
			requestNamespace:        namespaceName,
			expectedUseRegionalSTS:  true,
			expectedTokenExpiration: 10000,
		},
		{
			name:                    "Config map is set, but not service account",
			configMap:               k8sConfigMap,
			requestServiceAccount:   serviceAccountName,
			requestNamespace:        namespaceName,
			expectedUseRegionalSTS:  true,
			expectedTokenExpiration: 20000,
		},
		{
			name:                    "Both service account and config map is set, service account should take precedence",
			serviceAccount:          k8sServiceAccount,
			configMap:               k8sConfigMap,
			requestServiceAccount:   serviceAccountName,
			requestNamespace:        namespaceName,
			expectedUseRegionalSTS:  true,
			expectedTokenExpiration: 10000,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			cache := &serviceAccountCache{
				saCache:          map[string]*CacheResponse{},
				cmCache:          map[string]*CacheResponse{},
				defaultAudience:  "sts.amazonaws.com",
				annotationPrefix: "eks.amazonaws.com",
				webhookUsage:     prometheus.NewGauge(prometheus.GaugeOpts{}),
			}

			if tc.serviceAccount != nil {
				cache.addSA(tc.serviceAccount)
			}
			if tc.configMap != nil {
				cache.populateCacheFromCM(nil, tc.configMap)
			}

			useRegionalSTS, tokenExpiration := cache.GetCommonConfigurations(tc.requestServiceAccount, tc.requestNamespace)
			assert.Equal(t, tc.expectedUseRegionalSTS, useRegionalSTS)
			assert.Equal(t, tc.expectedTokenExpiration, tokenExpiration)
		})
	}
}
