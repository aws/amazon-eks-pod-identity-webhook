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
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type Entry struct {
	RoleARN         string
	Audience        string
	UseRegionalSTS  bool
	TokenExpiration int64
}

type Request struct {
	Name                string
	Namespace           string
	RequestNotification bool
}

func (r Request) CacheKey() string {
	return r.Namespace + "/" + r.Name
}

type Response struct {
	RoleARN         string
	Audience        string
	UseRegionalSTS  bool
	TokenExpiration int64
	FoundInCache    bool
	Notifier        <-chan struct{}
}

type ServiceAccountCache interface {
	Start(stop chan struct{})
	Get(request Request) Response
	GetCommonConfigurations(name, namespace string) (useRegionalSTS bool, tokenExpiration int64)
	// ToJSON returns cache contents as JSON string
	ToJSON() string
	Clear()
}

type serviceAccountCache struct {
	mu                     sync.RWMutex // guards cache
	saCache                map[string]*Entry
	cmCache                map[string]*Entry
	hasSynced              cache.InformerSynced
	clientset              kubernetes.Interface
	annotationPrefix       string
	defaultAudience        string
	defaultRegionalSTS     bool
	composeRoleArn         ComposeRoleArn
	defaultTokenExpiration int64
	webhookUsage           prometheus.Gauge
	notifications          *notifications
}

type ComposeRoleArn struct {
	Enabled bool

	AccountID string
	Partition string
	Region    string
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
// It will first look at the set of ServiceAccounts configured using annotations. If none is found and a notifier is
// requested, it will register a handler to be notified as soon as a ServiceAccount with given key is populated to the
// cache. Afterward it will check for a ServiceAccount configured through the pod-identity-webhook ConfigMap.
func (c *serviceAccountCache) Get(req Request) Response {
	result := Response{
		TokenExpiration: pkg.DefaultTokenExpiration,
	}
	klog.V(5).Infof("Fetching sa %s from cache", req.CacheKey())
	{
		var entry *Entry
		entry, result.Notifier = c.getSA(req)
		if entry != nil {
			result.FoundInCache = true
		}
		if entry != nil && entry.RoleARN != "" {
			result.RoleARN = entry.RoleARN
			result.Audience = entry.Audience
			result.UseRegionalSTS = entry.UseRegionalSTS
			result.TokenExpiration = entry.TokenExpiration
			return result
		}
	}
	{
		entry := c.getCM(req.Name, req.Namespace)
		if entry != nil {
			result.FoundInCache = true
			result.RoleARN = entry.RoleARN
			result.Audience = entry.Audience
			result.UseRegionalSTS = entry.UseRegionalSTS
			result.TokenExpiration = entry.TokenExpiration
			return result
		}
	}
	klog.V(5).Infof("Service account %s not found in cache", req.CacheKey())
	return result
}

// GetCommonConfigurations returns the common configurations that also applies to the new mutation method(i.e Container Credentials).
// The config file for the container credentials does not contain "TokenExpiration" or "UseRegionalSTS". For backward compatibility,
// Use these fields if they are set in the sa annotations or config map.
func (c *serviceAccountCache) GetCommonConfigurations(name, namespace string) (useRegionalSTS bool, tokenExpiration int64) {
	if entry, _ := c.getSA(Request{Name: name, Namespace: namespace, RequestNotification: false}); entry != nil {
		return entry.UseRegionalSTS, entry.TokenExpiration
	} else if entry := c.getCM(name, namespace); entry != nil {
		return entry.UseRegionalSTS, entry.TokenExpiration
	}
	return false, pkg.DefaultTokenExpiration
}

func (c *serviceAccountCache) getSA(req Request) (*Entry, <-chan struct{}) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.saCache[req.CacheKey()]
	if !ok && req.RequestNotification {
		klog.V(5).Infof("Service Account %s not found in cache, adding notification handler", req.CacheKey())
		return nil, c.notifications.create(req)
	}
	return entry, nil
}

func (c *serviceAccountCache) getCM(name, namespace string) *Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.cmCache[namespace+"/"+name]
	if !ok {
		return nil
	}
	return entry
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
	entry := &Entry{}

	arn, ok := sa.Annotations[c.annotationPrefix+"/"+pkg.RoleARNAnnotation]
	if ok {
		if !strings.Contains(arn, "arn:") && c.composeRoleArn.Enabled {
			arn = fmt.Sprintf("arn:%s:iam::%s:role/%s", c.composeRoleArn.Partition, c.composeRoleArn.AccountID, arn)
		}

		matched, err := regexp.Match(`^arn:aws[a-z0-9-]*:iam::\d{12}:role\/[\w-\/.@+=,]+$`, []byte(arn))
		if err != nil {
			klog.Errorf("Regex error: %v", err)
		} else if !matched {
			klog.Warningf("arn is invalid: %s", arn)
		}
		entry.RoleARN = arn
	}

	entry.Audience = c.defaultAudience
	if audience, ok := sa.Annotations[c.annotationPrefix+"/"+pkg.AudienceAnnotation]; ok {
		entry.Audience = audience
	}

	entry.UseRegionalSTS = c.defaultRegionalSTS
	if useRegionalStr, ok := sa.Annotations[c.annotationPrefix+"/"+pkg.UseRegionalSTSAnnotation]; ok {
		useRegional, err := strconv.ParseBool(useRegionalStr)
		if err != nil {
			klog.V(4).Infof("Ignoring service account %s/%s invalid value for disable-regional-sts annotation", sa.Namespace, sa.Name)
		} else {
			entry.UseRegionalSTS = useRegional
		}
	}

	entry.TokenExpiration = c.defaultTokenExpiration
	if tokenExpirationStr, ok := sa.Annotations[c.annotationPrefix+"/"+pkg.TokenExpirationAnnotation]; ok {
		if tokenExpiration, err := strconv.ParseInt(tokenExpirationStr, 10, 64); err != nil {
			klog.V(4).Infof("Found invalid value for token expiration, using %d seconds as default: %v", entry.TokenExpiration, err)
		} else {
			entry.TokenExpiration = pkg.ValidateMinTokenExpiration(tokenExpiration)
		}
	}
	c.webhookUsage.Set(1)

	c.setSA(sa.Name, sa.Namespace, entry)
}

func (c *serviceAccountCache) setSA(name, namespace string, entry *Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := namespace + "/" + name
	klog.V(5).Infof("Adding SA %q to SA cache: %+v", key, entry)
	c.saCache[key] = entry

	c.notifications.broadcast(key)
}

func (c *serviceAccountCache) setCM(name, namespace string, entry *Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	klog.V(5).Infof("Adding SA %s/%s to CM cache: %+v", namespace, name, entry)
	c.cmCache[namespace+"/"+name] = entry
}

func New(defaultAudience,
	prefix string,
	defaultRegionalSTS bool,
	defaultTokenExpiration int64,
	saInformer coreinformers.ServiceAccountInformer,
	cmInformer coreinformers.ConfigMapInformer,
	composeRoleArn ComposeRoleArn,
	SAGetter corev1.ServiceAccountsGetter,
) ServiceAccountCache {
	hasSynced := func() bool {
		if cmInformer != nil {
			return saInformer.Informer().HasSynced() && cmInformer.Informer().HasSynced()
		} else {
			return saInformer.Informer().HasSynced()
		}
	}

	// Rate limit to 10 concurrent requests against the API server.
	saFetchRequests := make(chan *Request, 10)
	c := &serviceAccountCache{
		saCache:                map[string]*Entry{},
		cmCache:                map[string]*Entry{},
		defaultAudience:        defaultAudience,
		annotationPrefix:       prefix,
		defaultRegionalSTS:     defaultRegionalSTS,
		composeRoleArn:         composeRoleArn,
		defaultTokenExpiration: defaultTokenExpiration,
		hasSynced:              hasSynced,
		webhookUsage:           webhookUsage,
		notifications:          newNotifications(saFetchRequests),
	}

	go func() {
		for req := range saFetchRequests {
			sa, err := fetchFromAPI(SAGetter, req)
			if err != nil {
				klog.Errorf("fetching SA: %s, but got error from API: %v", req.CacheKey(), err)
				continue
			}
			c.addSA(sa)
		}
	}()

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

func fetchFromAPI(getter corev1.ServiceAccountsGetter, req *Request) (*v1.ServiceAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()

	klog.V(5).Infof("fetching SA: %s", req.CacheKey())
	saList, err := getter.ServiceAccounts(req.Namespace).List(
		ctx,
		metav1.ListOptions{},
	)
	if err != nil {
		return nil, err
	}

	// Find the ServiceAccount
	for _, sa := range saList.Items {
		if sa.Name == req.Name {
			return &sa, nil

		}
	}
	return nil, fmt.Errorf("no SA found in namespace: %s", req.CacheKey())
}

func (c *serviceAccountCache) populateCacheFromCM(oldCM, newCM *v1.ConfigMap) error {
	if newCM.Name != "pod-identity-webhook" {
		return nil
	}
	newConfig := newCM.Data["config"]
	sas := make(map[string]*Entry)
	err := json.Unmarshal([]byte(newConfig), &sas)
	if err != nil {
		return fmt.Errorf("failed to unmarshal new config %q: %v", newConfig, err)
	}
	for key, entry := range sas {
		parts := strings.Split(key, "/")
		if entry.TokenExpiration == 0 {
			entry.TokenExpiration = c.defaultTokenExpiration
		}
		c.setCM(parts[1], parts[0], entry)
	}

	if oldCM != nil {
		oldConfig := oldCM.Data["config"]
		oldCache := make(map[string]*Entry)
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

func (c *serviceAccountCache) Clear() {
	c.saCache = map[string]*Entry{}
	c.cmCache = map[string]*Entry{}
}
