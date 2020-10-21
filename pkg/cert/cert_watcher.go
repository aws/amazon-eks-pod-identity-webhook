package cert

/*
  Provides base implementation to monitor a certificate for pending expiration, and trigger an
  implementation-defined method to renew it.

  Almost all of the actual lgoic below is copied from k8s.io/client-go/util/certificate/certificate_manager.go
  which implements the "watching for expiry" feature.
  The difference here is that our Reload method can do arbitrary task to obtain a new cert,
  where as the k8s certificate_manager only performs CSR request towards k8s API.
*/

import (
	"crypto/tls"
	"errors"
	"fmt"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"sync"
	"time"
)

type CertWatcher interface {
	Current() *tls.Certificate
	Start()
	Stop()
}

type CertProvider interface {
	Load() (*tls.Certificate, error)
}

type certWatcher struct {
	certAccessLock sync.RWMutex
	cert           *tls.Certificate

	lock    sync.Mutex
	stopCh  chan struct{}
	stopped bool

	provider CertProvider
}

func newCertWatcher(provider CertProvider) (*certWatcher, error) {
	c := &certWatcher{
		stopCh:   make(chan struct{}),
		provider: provider,
	}

	if ok, _ := c.reload(); !ok {
		return nil, errors.New("Configured certificate does not exist")
	}

	return c, nil
}

func (c *certWatcher) Current() *tls.Certificate {
	c.certAccessLock.RLock()
	defer c.certAccessLock.RUnlock()
	if c.cert != nil && c.cert.Leaf != nil && time.Now().After(c.cert.Leaf.NotAfter) {
		klog.V(2).Infof("Current certificate is expired.")
		return nil
	}
	return c.cert
}

func (c *certWatcher) reload() (bool, error) {
	newCert, err := c.provider.Load()
	if err != nil {
		klog.Errorf("reload failed: %v", err)
		return false, nil
	}

	if newCert.Leaf != nil && time.Now().After(newCert.Leaf.NotAfter) {
		klog.Errorf("reload failed, provided certificate is expired")
		return false, nil
	}

	c.certAccessLock.Lock()
	defer c.certAccessLock.Unlock()

	klog.Infof("Updating certificate, expires at %v", newCert.Leaf.NotAfter)
	c.cert = newCert

	return true, nil
}

func (c *certWatcher) Start() {
	go wait.Until(func() {
		deadline := c.nextRotationDeadline()
		if sleepInterval := deadline.Sub(time.Now()); sleepInterval > 0 {
			klog.V(2).Infof("Waiting %v for next certificate rotation", sleepInterval)

			timer := time.NewTimer(sleepInterval)
			defer timer.Stop()

			select {
			case <-timer.C:
				// unblock when deadline expires
				klog.V(2).Infof("Reloading certificate")
			}
		}

		backoff := wait.Backoff{
			Duration: 2 * time.Second,
			Factor:   2,
			Jitter:   0.1,
			Steps:    5,
		}
		if err := wait.ExponentialBackoff(backoff, c.reload); err != nil {
			utilruntime.HandleError(fmt.Errorf("Reached backoff limit, still unable to rotate certs: %v", err))
			wait.PollInfinite(32*time.Second, c.reload)
		}
	}, time.Second, c.stopCh)
}

func (c *certWatcher) Stop() {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.stopped {
		return
	}
	close(c.stopCh)
	c.stopped = true
}

// nextRotationDeadline returns a value for the threshold at which the
// current certificate should be rotated, 80%+/-10% of the expiration of the
// certificate.
func (c *certWatcher) nextRotationDeadline() time.Time {
	c.certAccessLock.RLock()
	defer c.certAccessLock.RUnlock()
	notAfter := c.cert.Leaf.NotAfter
	totalDuration := float64(notAfter.Sub(c.cert.Leaf.NotBefore))
	deadline := c.cert.Leaf.NotBefore.Add(jitteryDuration(totalDuration))

	klog.V(2).Infof("Certificate expiration is %v, rotation deadline is %v", notAfter, deadline)
	return deadline
}

// jitteryDuration uses some jitter to set the rotation threshold so each node
// will rotate at approximately 70-90% of the total lifetime of the
// certificate.  With jitter, if a number of nodes are added to a cluster at
// approximately the same time (such as cluster creation time), they won't all
// try to rotate certificates at the same time for the rest of the life of the
// cluster.
//
// This function is represented as a variable to allow replacement during testing.
var jitteryDuration = func(totalDuration float64) time.Duration {
	return wait.Jitter(time.Duration(totalDuration), 0.2) - time.Duration(totalDuration*0.3)
}
