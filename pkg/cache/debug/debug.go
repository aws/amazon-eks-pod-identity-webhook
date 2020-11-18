package debug

import (
	"fmt"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache"
	"k8s.io/klog"
	"net/http"
)

type Dumper struct {
	Cache cache.ServiceAccountCache
}

func (c *Dumper) Handle(w http.ResponseWriter, r *http.Request) {
	res := c.Cache.ToJSON()
	if _, err := w.Write([]byte(res)); err != nil {
		klog.Errorf("Can't dump cache contents: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}
