/*
  Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.

  Licensed under the Apache License, Version 2.0 (the "License").
  You may not use this file except in compliance with the License.
  A copy of the License is located at

      http://www.apache.org/licenses/LICENSE-2.0

  or in the "license" file accompanying this file. This file is distributed
  on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
  express or implied. See the License for the specific language governing
  permissions and limitations under the License.
*/

package cache

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

type CacheResponse struct {
	RoleARN         string
	Audience        string
	UseRegionalSTS  bool
	TokenExpiration int64
}

type ServiceAccountCache interface {
	Start(stop chan struct{})
	Get(name, namespace string) (role, aud string, useRegionalSTS bool, tokenExpiration int64)
	// ToJSON returns cache contents as JSON string
	ToJSON() string
}

type serviceAccountCache struct {
	mu                     sync.RWMutex // guards cache
	saCache                map[string]*CacheResponse
	cmCache                map[string]*CacheResponse
	hasSynced              cache.InformerSynced
	clientset              kubernetes.Interface
	annotationPrefix       string
	defaultAudience        string
	defaultRegionalSTS     bool
	defaultTokenExpiration int64
	webhookUsage           prometheus.Gauge
}

// We need a way to know if the webhook is used in a cluster.
// There are multiple ways to achieve that.
// We could keep track of the number of annotated service accounts, however we need some additional logic and refactoring to make sure the metric doesn't grow unbounded due to resync.
// We could also track the number of pod mutations, but that won't show us the usage until pods churn.
// This is a minimal way to know the usage. We can add more metrics for annotated service accounts and pod mutations as need arises.
var webhookUsage = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "pod_identity_webhook_used",
	Help: "Indicator to know pod identity webhook is used",
})

func init() {
	prometheus.MustRegister(webhookUsage)
}

// Get will return the cached configuration of the given ServiceAccount.
// It will first look at the set of ServiceAccounts configured using annotations. If none are found, it will look for any
// ServiceAccount configured through the pod-identity-webhook ConfigMap.
func (c *serviceAccountCache) Get(name, namespace string) (role, aud string, useRegionalSTS bool, tokenExpiration int64) {
	klog.V(5).Infof("Fetching sa %s/%s from cache", namespace, name)
	{
		resp := c.getSA(name, namespace)
		if resp != nil && resp.RoleARN != "" {
			return resp.RoleARN, resp.Audience, resp.UseRegionalSTS, resp.TokenExpiration
		}
	}
	{
		resp := c.getCM(name, namespace)
		if resp != nil {
			return resp.RoleARN, resp.Audience, resp.UseRegionalSTS, resp.TokenExpiration
		}
	}
	klog.V(5).Infof("Service account %s/%s not found in cache", namespace, name)
	return "", "", false, pkg.DefaultTokenExpiration
}

func (c *serviceAccountCache) getSA(name, namespace string) *CacheResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()
	resp, ok := c.saCache[namespace+"/"+name]
	if !ok {
		return nil
	}
	return resp
}

func (c *serviceAccountCache) getCM(name, namespace string) *CacheResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()
	resp, ok := c.cmCache[namespace+"/"+name]
	if !ok {
		return nil
	}
	return resp
}

func (c *serviceAccountCache) popSA(name, namespace string) {
	klog.V(5).Infof("Removing SA %s/%s from SA cache", namespace, name)
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.saCache, namespace+"/"+name)
}

func (c *serviceAccountCache) popCM(name, namespace string) {
	klog.V(5).Infof("Removing SA %s/%s from CM cache", namespace, name)
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cmCache, namespace+"/"+name)
}

// Log cache contents for debugginqg
func (c *serviceAccountCache) ToJSON() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	contents, err := json.MarshalIndent(c.saCache, "", " ")
	if err != nil {
		klog.Errorf("Json marshal error: %v", err.Error())
		return ""
	}
	return string(contents)
}

func (c *serviceAccountCache) addSA(sa *v1.ServiceAccount) {
	arn, ok := sa.Annotations[c.annotationPrefix+"/"+pkg.RoleARNAnnotation]
	resp := &CacheResponse{}
	if ok {
		resp.RoleARN = arn
		resp.Audience = c.defaultAudience
		if audience, ok := sa.Annotations[c.annotationPrefix+"/"+pkg.AudienceAnnotation]; ok {
			resp.Audience = audience
		}

		resp.UseRegionalSTS = c.defaultRegionalSTS
		if useRegionalStr, ok := sa.Annotations[c.annotationPrefix+"/"+pkg.UseRegionalSTSAnnotation]; ok {
			useRegional, err := strconv.ParseBool(useRegionalStr)
			if err != nil {
				klog.V(4).Infof("Ignoring service account %s/%s invalid value for disable-regional-sts annotation", sa.Namespace, sa.Name)
			} else {
				resp.UseRegionalSTS = useRegional
			}
		}

		resp.TokenExpiration = c.defaultTokenExpiration
		if tokenExpirationStr, ok := sa.Annotations[c.annotationPrefix+"/"+pkg.TokenExpirationAnnotation]; ok {
			if tokenExpiration, err := strconv.ParseInt(tokenExpirationStr, 10, 64); err != nil {
				klog.V(4).Infof("Found invalid value for token expiration, using %d seconds as default: %v", resp.TokenExpiration, err)
			} else {
				resp.TokenExpiration = pkg.ValidateMinTokenExpiration(tokenExpiration)
			}
		}
		c.webhookUsage.Set(1)
	}
	c.setSA(sa.Name, sa.Namespace, resp)
}

func (c *serviceAccountCache) setSA(name, namespace string, resp *CacheResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	klog.V(5).Infof("Adding SA %s/%s to SA cache: %+v", namespace, name, resp)
	c.saCache[namespace+"/"+name] = resp
}

func (c *serviceAccountCache) setCM(name, namespace string, resp *CacheResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	klog.V(5).Infof("Adding SA %s/%s to CM cache: %+v", namespace, name, resp)
	c.cmCache[namespace+"/"+name] = resp
}

func New(defaultAudience, prefix string, defaultRegionalSTS bool, defaultTokenExpiration int64, saInformer coreinformers.ServiceAccountInformer, cmInformer coreinformers.ConfigMapInformer) ServiceAccountCache {
	hasSynced := func() bool {
		if cmInformer != nil {
			return saInformer.Informer().HasSynced() && cmInformer.Informer().HasSynced()
		} else {
			return saInformer.Informer().HasSynced()
		}
	}
	c := &serviceAccountCache{
		saCache:                map[string]*CacheResponse{},
		cmCache:                map[string]*CacheResponse{},
		defaultAudience:        defaultAudience,
		annotationPrefix:       prefix,
		defaultRegionalSTS:     defaultRegionalSTS,
		defaultTokenExpiration: defaultTokenExpiration,
		hasSynced:              hasSynced,
		webhookUsage:           webhookUsage,
	}

	saInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				sa := obj.(*v1.ServiceAccount)
				c.addSA(sa)
			},
			DeleteFunc: func(obj interface{}) {
				sa, ok := obj.(*v1.ServiceAccount)
				if !ok {
					tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
					if !ok {
						utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %+v", obj))
						return
					}
					sa, ok = tombstone.Obj.(*v1.ServiceAccount)
					if !ok {
						utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a ServiceAccount %#v", obj))
						return
					}
				}
				c.popSA(sa.Name, sa.Namespace)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				sa := newObj.(*v1.ServiceAccount)
				c.addSA(sa)
			},
		},
	)
	if cmInformer != nil {
		cmInformer.Informer().AddEventHandler(
			cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					err := c.populateCacheFromCM(nil, obj.(*v1.ConfigMap))
					if err != nil {
						utilruntime.HandleError(err)
					}
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					err := c.populateCacheFromCM(oldObj.(*v1.ConfigMap), newObj.(*v1.ConfigMap))
					if err != nil {
						utilruntime.HandleError(err)
					}
				},
			},
		)
	}
	return c
}

func (c *serviceAccountCache) populateCacheFromCM(oldCM, newCM *v1.ConfigMap) error {
	if newCM.Name != "pod-identity-webhook" {
		return nil
	}
	newConfig := newCM.Data["config"]
	sas := make(map[string]*CacheResponse)
	err := json.Unmarshal([]byte(newConfig), &sas)
	if err != nil {
		return fmt.Errorf("failed to unmarshal new config %q: %v", newConfig, err)
	}
	for key, resp := range sas {
		parts := strings.Split(key, "/")
		if resp.TokenExpiration == 0 {
			resp.TokenExpiration = c.defaultTokenExpiration
		}
		c.setCM(parts[1], parts[0], resp)
	}

	if oldCM != nil {
		oldConfig := oldCM.Data["config"]
		oldCache := make(map[string]*CacheResponse)
		err := json.Unmarshal([]byte(oldConfig), &oldCache)
		if err != nil {
			return fmt.Errorf("failed to unmarshal old config %q: %v", oldConfig, err)
		}
		for key := range oldCache {
			if _, found := sas[key]; !found {
				parts := strings.Split(key, "/")
				c.popCM(parts[1], parts[0])
			}
		}
	}
	return nil
}

func (c *serviceAccountCache) start(stop chan struct{}) {

	if !cache.WaitForCacheSync(stop, c.hasSynced) {
		klog.Fatal("unable to sync serviceaccount cache!")
		return
	}

	<-stop
}

func (c *serviceAccountCache) Start(stop chan struct{}) {
	go c.start(stop)
}
