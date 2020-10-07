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
	"fmt"
	"strconv"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

type CacheResponse struct {
	RoleARN  string
	Audience string
	FSGroup  *int64
}

type ServiceAccountCache interface {
	Start()
	Get(name, namespace string) (role, aud string, fsGroup *int64)
}

type serviceAccountCache struct {
	mu               sync.RWMutex // guards cache
	cache            map[string]*CacheResponse
	store            cache.Store
	controller       cache.Controller
	clientset        kubernetes.Interface
	annotationPrefix string
	defaultAudience  string
}

func (c *serviceAccountCache) Get(name, namespace string) (role, aud string, fsGroup *int64) {
	klog.V(5).Infof("Fetching sa %s/%s from cache", namespace, name)
	resp := c.get(name, namespace)
	if resp == nil {
		return "", "", nil
	}
	// Immutable safety for the fsGroup *int64 in the cache
	if resp.FSGroup != nil {
		fsGroup = new(int64)
		*fsGroup = *resp.FSGroup
	}
	return resp.RoleARN, resp.Audience, fsGroup
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

func (c *serviceAccountCache) addSA(sa *v1.ServiceAccount) {
	arn, ok := sa.Annotations[c.annotationPrefix+"/role-arn"]
	resp := &CacheResponse{}
	if ok {
		resp.RoleARN = arn
		if audience, ok := sa.Annotations[c.annotationPrefix+"/audience"]; ok {
			resp.Audience = audience
		} else {
			resp.Audience = c.defaultAudience
		}
		if fsgStr, ok := sa.Annotations[c.annotationPrefix+"/fs-group"]; ok {
			if fsgInt, err := strconv.ParseInt(fsgStr, 10, 64); err == nil {
				resp.FSGroup = &fsgInt
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

func New(defaultAudience, prefix string, clientset kubernetes.Interface) ServiceAccountCache {
	c := &serviceAccountCache{
		cache:            map[string]*CacheResponse{},
		defaultAudience:  defaultAudience,
		annotationPrefix: prefix,
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
