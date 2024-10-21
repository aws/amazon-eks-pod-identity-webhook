package cache

import (
	"sync"

	"k8s.io/klog/v2"

	"github.com/prometheus/client_golang/prometheus"
)

var notificationUsage = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "pod_identity_cache_notifications",
		Help: "Counter of SA notifications",
	},
	[]string{"method"},
)

func init() {
	prometheus.MustRegister(notificationUsage)
}

type notifications struct {
	handlers      map[string]chan struct{}
	mu            sync.Mutex
	fetchRequests chan<- *Request
}

func newNotifications(saFetchRequests chan<- *Request) *notifications {
	return &notifications{
		handlers:      map[string]chan struct{}{},
		fetchRequests: saFetchRequests,
	}
}

func (n *notifications) create(req Request) <-chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()

	notificationUsage.WithLabelValues("used").Inc()
	notifier, found := n.handlers[req.CacheKey()]
	if !found {
		notifier = make(chan struct{})
		n.handlers[req.CacheKey()] = notifier
		notificationUsage.WithLabelValues("created").Inc()
		n.fetchRequests <- &req
	}
	return notifier
}

func (n *notifications) broadcast(key string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if handler, found := n.handlers[key]; found {
		klog.V(5).Infof("Notifying handlers for %q", key)
		notificationUsage.WithLabelValues("broadcast").Inc()
		close(handler)
		delete(n.handlers, key)
	}
}
