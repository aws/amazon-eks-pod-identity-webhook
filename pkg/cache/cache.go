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
	"sync"
	"time"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
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
	Start()
	Get(name, namespace string) (role, aud string, useRegionalSTS bool, tokenExpiration int64)
	// ToJSON returns cache contents as JSON string
	ToJSON() string
}

type serviceAccountCache struct {
	mu                     sync.RWMutex // guards cache
	cache                  map[string]*CacheResponse
	store                  cache.Store
	controller             cache.Controller
	clientset              kubernetes.Interface
	annotationPrefix       string
	defaultAudience        string
	defaultRegionalSTS     bool
	defaultTokenExpiration int64
}

func (c *serviceAccountCache) Get(name, namespace string) (role, aud string, useRegionalSTS bool, tokenExpiration int64) {
	klog.V(5).Infof("Fetching sa %s/%s from cache", namespace, name)
	resp := c.get(name, namespace)
	if resp == nil {
		klog.V(4).Infof("Service account %s/%s not found in cache", namespace, name)
		return "", "", false, pkg.DefaultTokenExpiration
	}
	return resp.RoleARN, resp.Audience, resp.UseRegionalSTS, resp.TokenExpiration
}

func (c *serviceAccountCache) get(name, namespace string) *CacheResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()
	resp, ok := c.cache[namespace+"/"+name]
	if !ok {
		return nil
	}
	return resp
}

func (c *serviceAccountCache) pop(name, namespace string) {
	klog.V(5).Infof("Removing sa %s/%s from cache", namespace, name)
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, namespace+"/"+name)
}

// Log cache contents for debugginqg
func (c *serviceAccountCache) ToJSON() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	contents, err := json.MarshalIndent(c.cache, "", " ")
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
		if disableRegionalStr, ok := sa.Annotations[c.annotationPrefix+"/"+pkg.UseRegionalSTSAnnotation]; ok {
			disableRegional, err := strconv.ParseBool(disableRegionalStr)
			if err != nil {
				klog.V(4).Infof("Ignoring service account %s/%s invalid value for disable-regional-sts annotation", sa.Namespace, sa.Name)
			} else {
				resp.UseRegionalSTS = !disableRegional
			}
		}

		resp.TokenExpiration = c.defaultTokenExpiration
		if tokenExpirationStr, ok := sa.Annotations[c.annotationPrefix + "/" + pkg.TokenExpirationAnnotation]; ok {
			if tokenExpiration, err := strconv.ParseInt(tokenExpirationStr, 10, 64); err != nil {
				klog.V(4).Infof("Found invalid value for token expiration, using %d seconds as default: %v", resp.TokenExpiration, err)
			} else {
				resp.TokenExpiration = pkg.ValidateMinTokenExpiration(tokenExpiration)
			}
		}
	}
	klog.V(5).Infof("Adding sa %s/%s to cache", sa.Name, sa.Namespace)
	c.set(sa.Name, sa.Namespace, resp)
}

func (c *serviceAccountCache) set(name, namespace string, resp *CacheResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[namespace+"/"+name] = resp
}

func New(defaultAudience, prefix string, defaultRegionalSTS bool, defaultTokenExpiration int64, clientset kubernetes.Interface) ServiceAccountCache {
	c := &serviceAccountCache{
		cache:                  map[string]*CacheResponse{},
		defaultAudience:        defaultAudience,
		annotationPrefix:       prefix,
		defaultRegionalSTS:     defaultRegionalSTS,
		defaultTokenExpiration: defaultTokenExpiration,
	}

	saListWatcher := cache.NewListWatchFromClient(
		clientset.CoreV1().RESTClient(),
		"serviceaccounts",
		v1.NamespaceAll,
		fields.Everything(),
	)

	c.store, c.controller = cache.NewInformer(
		saListWatcher,
		&v1.ServiceAccount{},
		time.Second*60,
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
				c.pop(sa.Name, sa.Namespace)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				sa := newObj.(*v1.ServiceAccount)
				c.addSA(sa)
			},
		},
	)
	return c
}

func (c *serviceAccountCache) start() {
	// Populate the cache
	err := cache.ListAll(c.store, labels.Everything(), func(obj interface{}) {
		sa := obj.(*v1.ServiceAccount)
		c.addSA(sa)
	})
	if err != nil {
		klog.Errorf("Error fetching service accounts: %v", err.Error())
		return
	}

	stop := make(chan struct{})
	defer close(stop)
	go c.controller.Run(stop)
	// Wait forever
	select {}
}

func (c *serviceAccountCache) Start() {
	go c.start()
}
