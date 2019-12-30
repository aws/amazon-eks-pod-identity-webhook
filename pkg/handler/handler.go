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

package handler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache"
	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/apis/core/v1"
)

func init() {
	_ = corev1.AddToScheme(runtimeScheme)
	_ = admissionregistrationv1beta1.AddToScheme(runtimeScheme)
	_ = v1.AddToScheme(runtimeScheme)
}

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

// ModifierOpt is an option type for setting up a Modifier
type ModifierOpt func(*Modifier)

// WithServiceAccountCache sets the modifiers cache
func WithServiceAccountCache(c cache.ServiceAccountCache) ModifierOpt {
	return func(m *Modifier) { m.Cache = c }
}

// WithMountPath sets the modifier mountPath
func WithMountPath(mountpath string) ModifierOpt {
	return func(m *Modifier) { m.MountPath = mountpath }
}

// WithExpiration sets the modifier expiration
func WithExpiration(exp int64) ModifierOpt {
	return func(m *Modifier) { m.Expiration = exp }
}

// NewModifier returns a Modifier with default values
func NewModifier(opts ...ModifierOpt) *Modifier {

	mod := &Modifier{
		MountPath:  "/var/run/secrets/eks.amazonaws.com/serviceaccount",
		Expiration: 86400,
		volName:    "aws-iam-token",
		tokenName:  "token",
	}
	for _, opt := range opts {
		opt(mod)
	}

	return mod
}

// Modifier holds configuration values for pod modifications
type Modifier struct {
	Expiration int64
	MountPath  string
	Cache      cache.ServiceAccountCache
	volName    string
	tokenName  string
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func addEnvToContainer(container *corev1.Container, mountPath, tokenFilePath, volName, roleName string) {
	reservedKeys := map[string]string{
		"AWS_ROLE_ARN":                "",
		"AWS_WEB_IDENTITY_TOKEN_FILE": "",
	}
	for _, env := range container.Env {
		if _, ok := reservedKeys[env.Name]; ok {
			// Skip if any env vars are already present
			return
		}
	}

	for _, vol := range container.VolumeMounts {
		if vol.Name == volName {
			// Skip if volume is already present
			return
		}
	}

	env := container.Env
	env = append(env, corev1.EnvVar{
		Name:  "AWS_ROLE_ARN",
		Value: roleName,
	})

	env = append(env, corev1.EnvVar{
		Name:  "AWS_WEB_IDENTITY_TOKEN_FILE",
		Value: tokenFilePath,
	})
	container.Env = env
	container.VolumeMounts = append(
		container.VolumeMounts,
		corev1.VolumeMount{
			Name:      volName,
			ReadOnly:  true,
			MountPath: mountPath,
		},
	)
}

func (m *Modifier) updatePodSpec(pod *corev1.Pod, roleName, audience string) []patchOperation {
	// return early if volume already exists
	for _, vol := range pod.Spec.Volumes {
		if vol.Name == m.volName {
			return nil
		}
	}

	tokenFilePath := filepath.Join(m.MountPath, m.tokenName)

	betaNodeSelector, _ := pod.Spec.NodeSelector["beta.kubernetes.io/os"]
	nodeSelector, _ := pod.Spec.NodeSelector["kubernetes.io/os"]
	if (betaNodeSelector == "windows") || nodeSelector == "windows" {
		// Convert the unix file path to a windows file path
		// Eg. /var/run/secrets/eks.amazonaws.com/serviceaccount/token to
		//     C:\var\run\secrets\eks.amazonaws.com\serviceaccount\token
		tokenFilePath = "C:" + strings.Replace(tokenFilePath, `/`, `\`, -1)
	}

	var initContainers = []corev1.Container{}
	for i := range pod.Spec.InitContainers {
		container := pod.Spec.InitContainers[i]
		addEnvToContainer(&container, m.MountPath, tokenFilePath, m.volName, roleName)
		initContainers = append(initContainers, container)
	}
	var containers = []corev1.Container{}
	for i := range pod.Spec.Containers {
		container := pod.Spec.Containers[i]
		addEnvToContainer(&container, m.MountPath, tokenFilePath, m.volName, roleName)
		containers = append(containers, container)
	}

	volume := corev1.Volume{
		m.volName,
		corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources: []corev1.VolumeProjection{
					corev1.VolumeProjection{
						ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
							Audience:          audience,
							ExpirationSeconds: &m.Expiration,
							Path:              m.tokenName,
						},
					},
				},
			},
		},
	}

	patch := []patchOperation{
		patchOperation{
			Op:    "add",
			Path:  "/spec/volumes/0",
			Value: volume,
		},
	}

	if pod.Spec.Volumes == nil {
		patch = []patchOperation{
			patchOperation{
				Op:   "add",
				Path: "/spec/volumes",
				Value: []corev1.Volume{
					volume,
				},
			},
		}
	}

	patch = append(patch, patchOperation{
		Op:    "add",
		Path:  "/spec/containers",
		Value: containers,
	})

	if len(initContainers) > 0 {
		patch = append(patch, patchOperation{
			Op:    "add",
			Path:  "/spec/initContainers",
			Value: initContainers,
		})
	}
	return patch
}

// MutatePod takes a AdmissionReview, mutates the pod, and returns an AdmissionResponse
func (m *Modifier) MutatePod(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	badRequest := &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: "bad content",
		},
	}
	if ar == nil {
		return badRequest
	}
	req := ar.Request
	if req == nil {
		return badRequest
	}

	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		klog.Errorf("Could not unmarshal raw object: %v", err)
		klog.Errorf("Object: %v", string(req.Object.Raw))
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	pod.Namespace = req.Namespace

	podRole, audience := m.Cache.Get(pod.Spec.ServiceAccountName, pod.Namespace)

	// determine whether to perform mutation
	if podRole == "" {
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	patchBytes, err := json.Marshal(m.updatePodSpec(&pod, podRole, audience))
	if err != nil {
		klog.Errorf("Error marshaling pod update: %v", err.Error())
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	return &v1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *v1beta1.PatchType {
			pt := v1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

// Handle handles pod modification requests
func (m *Modifier) Handle(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		klog.Errorf("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		klog.Errorf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		klog.Errorf("Can't decode body: %v", err)
		admissionResponse = &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		admissionResponse = m.MutatePod(&ar)
	}

	admissionReview := v1beta1.AdmissionReview{}
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse
		if ar.Request != nil {
			admissionReview.Response.UID = ar.Request.UID
		}
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
