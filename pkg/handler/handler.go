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
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/containercredentials"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache"
	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog/v2"
)

func init() {
	_ = corev1.AddToScheme(runtimeScheme)
	_ = admissionregistrationv1beta1.AddToScheme(runtimeScheme)
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

// WithContainerCredentialsConfig sets the modifier ContainerCredentialsConfig
func WithContainerCredentialsConfig(config containercredentials.Config) ModifierOpt {
	return func(m *Modifier) { m.ContainerCredentialsConfig = config }
}

// WithMountPath sets the modifier mountPath
func WithMountPath(mountpath string) ModifierOpt {
	return func(m *Modifier) { m.MountPath = mountpath }
}

// WithRegion sets the modifier region
func WithRegion(region string) ModifierOpt {
	return func(m *Modifier) { m.Region = region }
}

// WithAnnotationDomain adds an annotation domain
func WithAnnotationDomain(domain string) ModifierOpt {
	return func(m *Modifier) { m.AnnotationDomain = domain }
}

// WithSALookupGraceTime sets the grace time to wait for service accounts to appear in cache
func WithSALookupGraceTime(saLookupGraceTime time.Duration) ModifierOpt {
	return func(m *Modifier) { m.saLookupGraceTime = saLookupGraceTime }

}

// NewModifier returns a Modifier with default values
func NewModifier(opts ...ModifierOpt) *Modifier {
	mod := &Modifier{
		AnnotationDomain: "eks.amazonaws.com",
		MountPath:        "/var/run/secrets/eks.amazonaws.com/serviceaccount",
		volName:          "aws-iam-token",
		tokenName:        "token",
	}
	for _, opt := range opts {
		opt(mod)
	}

	return mod
}

// Modifier holds configuration values for pod modifications
type Modifier struct {
	AnnotationDomain           string
	MountPath                  string
	Region                     string
	Cache                      cache.ServiceAccountCache
	ContainerCredentialsConfig containercredentials.Config
	volName                    string
	tokenName                  string
	saLookupGraceTime          time.Duration
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

type podPatchConfig struct {
	ContainersToSkip                map[string]bool
	TokenExpiration                 int64
	UseRegionalSTS                  bool
	Audience                        string
	MountPath                       string
	VolumeName                      string
	TokenPath                       string
	WebIdentityPatchConfig          *webIdentityPatchConfig
	ContainerCredentialsPatchConfig *containercredentials.PatchConfig
}

type webIdentityPatchConfig struct {
	RoleArn string
}

func logContext(podName, podGenerateName, serviceAccountName, namespace string) string {
	name := podName
	if len(podName) == 0 {
		name = podGenerateName
	}
	return fmt.Sprintf("Pod=%s, "+
		"ServiceAccount=%s, "+
		"Namespace=%s", name, serviceAccountName, namespace)
}

// getContainersToSkip returns the containers of a pod to skip mutating
func getContainersToSkip(annotationDomain string, pod *corev1.Pod) map[string]bool {
	skippedNames := map[string]bool{}
	skipContainersKey := annotationDomain + "/" + pkg.SkipContainersAnnotation
	if value, ok := pod.Annotations[skipContainersKey]; ok {
		r := csv.NewReader(strings.NewReader(value))
		// error means we don't skip any
		podNames, err := r.Read()
		if err != nil {
			klog.Infof("Could not parse skip containers annotation on pod %s/%s: %v", pod.Namespace, pod.Name, err)
			return skippedNames
		}
		for _, name := range podNames {
			skippedNames[name] = true
		}
	}
	return skippedNames
}

func (m *Modifier) addEnvToContainer(container *corev1.Container, tokenFilePath string, patchConfig *podPatchConfig) bool {
	var (
		webIdentityKeysDefined          bool
		containerCredentialsKeysDefined bool
		regionKeyDefined                bool
		regionalStsKeyDefined           bool
	)
	webIdentityKeys := map[string]string{
		"AWS_ROLE_ARN":                "",
		"AWS_WEB_IDENTITY_TOKEN_FILE": "",
	}
	containerCredentialsKeys := map[string]string{
		pkg.AwsEnvVarContainerCredentialsFullUri:     "",
		pkg.AwsEnvVarContainerAuthorizationTokenFile: "",
	}
	awsRegionKeys := map[string]string{
		"AWS_REGION":         "",
		"AWS_DEFAULT_REGION": "",
	}
	stsKey := "AWS_STS_REGIONAL_ENDPOINTS"
	for _, env := range container.Env {
		if _, ok := webIdentityKeys[env.Name]; ok {
			klog.V(4).Infof("Web identity env variable %s is already defined in the pod spec", env)
			webIdentityKeysDefined = true
		}
		if _, ok := containerCredentialsKeys[env.Name]; ok {
			klog.V(4).Infof("Container credential env variable %s is already defined in the pod spec", env)
			containerCredentialsKeysDefined = true
		}
		if _, ok := awsRegionKeys[env.Name]; ok {
			// Don't set both region keys if any region key is already set
			klog.V(4).Infof("AWS Region env variable %s is already defined in the pod spec", env)
			regionKeyDefined = true
		}
		if env.Name == stsKey {
			klog.V(4).Infof("AWS STS env variable %s is already defined in the pod spec", env)
			regionalStsKeyDefined = true
		}
	}

	if ((patchConfig.WebIdentityPatchConfig != nil && webIdentityKeysDefined) ||
		(patchConfig.ContainerCredentialsPatchConfig != nil && containerCredentialsKeysDefined)) &&
		regionKeyDefined && regionalStsKeyDefined {
		klog.V(4).Infof("Container %s has necessary env variables already present", container.Name)
		return false
	}

	changed := false
	env := container.Env

	if !regionalStsKeyDefined && patchConfig.UseRegionalSTS {
		env = append(env, corev1.EnvVar{
			Name:  stsKey,
			Value: "regional",
		})
		changed = true
	}

	if !regionKeyDefined && m.Region != "" {
		env = append(env, corev1.EnvVar{
			Name:  "AWS_DEFAULT_REGION",
			Value: m.Region,
		}, corev1.EnvVar{
			Name:  "AWS_REGION",
			Value: m.Region,
		})
		changed = true
	}

	if patchConfig.ContainerCredentialsPatchConfig != nil {
		if !containerCredentialsKeysDefined {
			env = append(env, corev1.EnvVar{
				Name:  pkg.AwsEnvVarContainerCredentialsFullUri,
				Value: patchConfig.ContainerCredentialsPatchConfig.FullUri,
			})
			env = append(env, corev1.EnvVar{
				Name:  pkg.AwsEnvVarContainerAuthorizationTokenFile,
				Value: tokenFilePath,
			})
			changed = true
		}
	} else if patchConfig.WebIdentityPatchConfig != nil {
		if !webIdentityKeysDefined {
			env = append(env, corev1.EnvVar{
				Name:  "AWS_ROLE_ARN",
				Value: patchConfig.WebIdentityPatchConfig.RoleArn,
			})
			env = append(env, corev1.EnvVar{
				Name:  "AWS_WEB_IDENTITY_TOKEN_FILE",
				Value: tokenFilePath,
			})
			changed = true
		}
	}

	container.Env = env

	volExists := false
	for _, vol := range container.VolumeMounts {
		if vol.Name == patchConfig.VolumeName {
			volExists = true
		}
	}

	if !volExists {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      patchConfig.VolumeName,
			ReadOnly:  true,
			MountPath: patchConfig.MountPath,
		})
		changed = true
	}
	return changed
}

// parsePodAnnotations parses the pod annotations that can influence mutation:
// - tokenExpiration. Overrides the given service account annotation/flag-level
// setting.
// - containersToSkip. A Pod specific setting since certain containers within a
// specific pod might need to be opted-out of mutation
func (m *Modifier) parsePodAnnotations(pod *corev1.Pod, serviceAccountTokenExpiration int64) (int64, map[string]bool) {
	// override serviceaccount annotation/flag token expiration with pod
	// annotation if present
	tokenExpiration := serviceAccountTokenExpiration
	expirationKey := m.AnnotationDomain + "/" + pkg.TokenExpirationAnnotation
	if expirationStr, ok := pod.Annotations[expirationKey]; ok {
		if expiration, err := strconv.ParseInt(expirationStr, 10, 64); err != nil {
			klog.V(4).Infof("Found invalid value for token expiration, using %d seconds as default: %v", serviceAccountTokenExpiration, err)
		} else {
			tokenExpiration = pkg.ValidateMinTokenExpiration(expiration)
		}
	}

	containersToSkip := getContainersToSkip(m.AnnotationDomain, pod)

	return tokenExpiration, containersToSkip
}

// getPodSpecPatch gets the patch operation to be applied to the given Pod
func (m *Modifier) getPodSpecPatch(pod *corev1.Pod, patchConfig *podPatchConfig) ([]patchOperation, bool) {
	tokenFilePath := filepath.Join(patchConfig.MountPath, patchConfig.TokenPath)

	betaNodeSelector, _ := pod.Spec.NodeSelector["beta.kubernetes.io/os"]
	nodeSelector, _ := pod.Spec.NodeSelector["kubernetes.io/os"]
	if (betaNodeSelector == "windows") || nodeSelector == "windows" {
		// Convert the unix file path to a windows file path
		// Eg. /var/run/secrets/eks.amazonaws.com/serviceaccount/token to
		//     C:\var\run\secrets\eks.amazonaws.com\serviceaccount\token
		tokenFilePath = "C:" + strings.Replace(tokenFilePath, `/`, `\`, -1)
	}

	var changed bool

	var initContainers = []corev1.Container{}
	for i := range pod.Spec.InitContainers {
		container := pod.Spec.InitContainers[i]
		if _, ok := patchConfig.ContainersToSkip[container.Name]; ok {
			klog.V(4).Infof("Container %s was annotated to be skipped", container.Name)
		} else if m.addEnvToContainer(&container, tokenFilePath, patchConfig) {
			changed = true
		}
		initContainers = append(initContainers, container)
	}

	var containers = []corev1.Container{}
	for i := range pod.Spec.Containers {
		container := pod.Spec.Containers[i]
		if _, ok := patchConfig.ContainersToSkip[container.Name]; ok {
			klog.V(4).Infof("Container %s was annotated to be skipped", container.Name)
		} else if m.addEnvToContainer(&container, tokenFilePath, patchConfig) {
			changed = true
		}
		containers = append(containers, container)
	}

	volume := corev1.Volume{
		Name: patchConfig.VolumeName,
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources: []corev1.VolumeProjection{
					{
						ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
							Audience:          patchConfig.Audience,
							ExpirationSeconds: &patchConfig.TokenExpiration,
							Path:              patchConfig.TokenPath,
						},
					},
				},
			},
		},
	}

	patch := []patchOperation{}

	// skip adding volume if it already exists
	volExists := false
	for _, vol := range pod.Spec.Volumes {
		if vol.Name == patchConfig.VolumeName {
			volExists = true
		}
	}

	if !volExists {
		volPatch := patchOperation{
			Op:    "add",
			Path:  "/spec/volumes/0",
			Value: volume,
		}

		if pod.Spec.Volumes == nil {
			volPatch = patchOperation{
				Op:   "add",
				Path: "/spec/volumes",
				Value: []corev1.Volume{
					volume,
				},
			}
		}

		patch = append(patch, volPatch)
		changed = true
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
	return patch, changed
}

// buildPodPatchConfig reads configurations from multiples data sources and builds a merged podPatchConfig.
// Data sources include: Cache, ContainerCredentialsConfig, and pod's annotations.
//
// Some mutation parameters can be overridden via pod or serviceaccount
// annotations. The serviceaccount cache already parsed the serviceaccount
// annotations and flags such that annotations take precedence.
// audience:        serviceaccount annotation > flag
// regionalSTS:     serviceaccount annotation > flag
// tokenExpiration: pod annotation > serviceaccount annotation > flag
func (m *Modifier) buildPodPatchConfig(pod *corev1.Pod) *podPatchConfig {
	// Container credentials method takes precedence
	containerCredentialsPatchConfig := m.ContainerCredentialsConfig.Get(pod.Namespace, pod.Spec.ServiceAccountName)
	if containerCredentialsPatchConfig != nil {
		regionalSTS, tokenExpiration := m.Cache.GetCommonConfigurations(pod.Spec.ServiceAccountName, pod.Namespace)
		tokenExpiration, containersToSkip := m.parsePodAnnotations(pod, tokenExpiration)

		webhookPodCount.WithLabelValues("container_credentials").Inc()

		return &podPatchConfig{
			ContainersToSkip:                containersToSkip,
			TokenExpiration:                 tokenExpiration,
			UseRegionalSTS:                  regionalSTS,
			Audience:                        containerCredentialsPatchConfig.Audience,
			MountPath:                       containerCredentialsPatchConfig.MountPath,
			VolumeName:                      containerCredentialsPatchConfig.VolumeName,
			TokenPath:                       containerCredentialsPatchConfig.TokenPath,
			WebIdentityPatchConfig:          nil,
			ContainerCredentialsPatchConfig: containerCredentialsPatchConfig,
		}
	}

	// Use the STS WebIdentity method if set
	handler := make(chan any, 1)
	roleArn, audience, regionalSTS, tokenExpiration, found := m.Cache.GetOrNotify(pod.Spec.ServiceAccountName, pod.Namespace, handler)
	key := pod.Namespace + "/" + pod.Spec.ServiceAccountName
	if !found && m.saLookupGraceTime > 0 {
		klog.Warningf("Service account %q not found in the cache. Waiting up to %s to be notified", key, m.saLookupGraceTime)
		select {
		case <-handler:
			roleArn, audience, regionalSTS, tokenExpiration, found = m.Cache.Get(pod.Spec.ServiceAccountName, pod.Namespace)
			if !found {
				klog.Warningf("Service account %q not found in the cache after being notified. Not mutating.", key)
				return nil
			}
		case <-time.After(m.saLookupGraceTime):
			klog.Warningf("Service account %q not found in the cache after %s. Not mutating.", key, m.saLookupGraceTime)
			return nil
		}
	}
	klog.V(5).Infof("Value of roleArn after after cache retrieval for service account %q: %s", key, roleArn)
	if roleArn != "" {
		tokenExpiration, containersToSkip := m.parsePodAnnotations(pod, tokenExpiration)

		webhookPodCount.WithLabelValues("sts_web_identity").Inc()

		return &podPatchConfig{
			ContainersToSkip:                containersToSkip,
			TokenExpiration:                 tokenExpiration,
			UseRegionalSTS:                  regionalSTS,
			Audience:                        audience,
			MountPath:                       m.MountPath,
			VolumeName:                      m.volName,
			TokenPath:                       m.tokenName,
			WebIdentityPatchConfig:          &webIdentityPatchConfig{RoleArn: roleArn},
			ContainerCredentialsPatchConfig: nil,
		}
	}

	// No mutations needed
	return nil
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

	patchConfig := m.buildPodPatchConfig(&pod)
	if patchConfig == nil {
		klog.V(4).Infof("Pod was not mutated. Reason: "+
			"Service account did not have the right annotations or was not found in the cache. %s", logContext(pod.Name, pod.GenerateName, pod.Spec.ServiceAccountName, pod.Namespace))
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	patch, changed := m.getPodSpecPatch(&pod, patchConfig)
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		klog.Errorf("Error marshaling pod update: %v", err.Error())
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	// TODO: klog structured logging can make this better
	if changed {
		klog.V(3).Infof("Pod was mutated. %s", logContext(pod.Name, pod.GenerateName, pod.Spec.ServiceAccountName, pod.Namespace))
	} else {
		klog.V(3).Infof("Pod was not mutated. Reason: "+
			"Required volume mounts and env variables were already present. %s", logContext(pod.Name, pod.GenerateName, pod.Spec.ServiceAccountName, pod.Namespace))
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

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		klog.Errorf("Content-Type=%s, expected application/json", contentType)
		http.Error(w, "Invalid Content-Type, expected `application/json`", http.StatusUnsupportedMediaType)
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
