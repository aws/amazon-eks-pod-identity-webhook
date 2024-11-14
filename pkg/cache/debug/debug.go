package debug

import (
	"encoding/json"
	"fmt"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/klog/v2"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (c *Dumper) Clear(w http.ResponseWriter, r *http.Request) {
	c.Cache.Clear()
}

func (c *Dumper) InternalServerError(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "test error", http.StatusInternalServerError)
}

func (c *Dumper) Deny(w http.ResponseWriter, r *http.Request) {
	admissionReview := &v1beta1.AdmissionReview{
		Response: &v1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: "Test deny message",
			},
		},
	}
	resp, err := json.Marshal(admissionReview)
	if err != nil {
		klog.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}
	if _, err := w.Write(resp); err != nil {
		klog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}
