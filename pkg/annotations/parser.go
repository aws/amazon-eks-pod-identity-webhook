/*
Copyright 2020 Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License").
You may not use this file except in compliance with the License.
A copy of the License is located at

	http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed
on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
express or implied. See the License for the specific language governing
permissions and limitations under the License.
*/

package annotations

import (
	"encoding/csv"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

type PodAnnotations struct {
	tokenExpiration     *int64
	containersToSkip    map[string]bool
	saLookupGracePeriod *time.Duration
}

func (a *PodAnnotations) GetContainersToSkip() map[string]bool {
	return a.containersToSkip
}

func (a *PodAnnotations) GetTokenExpiration(fallback int64) int64 {
	if a.tokenExpiration == nil {
		return fallback
	} else {
		return *a.tokenExpiration
	}
}

func (a *PodAnnotations) GetSALookupGracePeriod(fallback time.Duration) time.Duration {
	if a.saLookupGracePeriod == nil {
		return fallback
	} else {
		return *a.saLookupGracePeriod
	}
}

// parsePodAnnotations parses the pod annotations that can influence mutation:
// - tokenExpiration. Overrides the given service account annotation/flag-level
// setting.
// - containersToSkip. A Pod specific setting since certain containers within a
// specific pod might need to be opted-out of mutation
func ParsePodAnnotations(pod *corev1.Pod, annotationDomain string) *PodAnnotations {
	return &PodAnnotations{
		tokenExpiration:     parseTokenExpiration(annotationDomain, pod),
		containersToSkip:    parseContainersToSkip(annotationDomain, pod),
		saLookupGracePeriod: parseSALookupGracePeriod(annotationDomain, pod),
	}
}

// parseContainersToSkip returns the containers of a pod to skip mutating
func parseContainersToSkip(annotationDomain string, pod *corev1.Pod) map[string]bool {
	skippedNames := map[string]bool{}
	skipContainersKey := annotationDomain + "/" + SkipContainersAnnotation

	value, ok := pod.Annotations[skipContainersKey]
	if !ok {
		return nil
	}
	r := csv.NewReader(strings.NewReader(value))
	// error means we don't skip any
	podNames, err := r.Read()
	if err != nil {
		klog.Infof("Could not parse skip containers annotation on pod %s/%s: %v", pod.Namespace, pod.Name, err)
		return nil
	}
	for _, name := range podNames {
		skippedNames[name] = true
	}
	return skippedNames
}

func parseTokenExpiration(annotationDomain string, pod *corev1.Pod) *int64 {
	expirationKey := annotationDomain + "/" + TokenExpirationAnnotation
	expirationStr, ok := pod.Annotations[expirationKey]
	if !ok {
		return nil
	}

	expiration, err := strconv.ParseInt(expirationStr, 10, 64)
	if err != nil {
		klog.V(4).Infof("Found invalid value for token expiration on the pod annotation: %s, falling back to the default: %v", expirationStr, err)
		return nil
	}

	val := pkg.ValidateMinTokenExpiration(expiration)
	return &val
}

func parseSALookupGracePeriod(annotationDomain string, pod *corev1.Pod) *time.Duration {
	gracePeriodKey := annotationDomain + "/" + SALookupGracePeriod

	gracePeriodStr, ok := pod.Annotations[gracePeriodKey]
	if !ok {
		return nil
	}

	gracePeriod, err := strconv.ParseInt(gracePeriodStr, 10, 64)
	if err != nil {
		klog.V(4).Infof("Found invalid value for SA lookup grace period on the pod annotation: %s, falling back to the default: %v", gracePeriodStr, err)
		return nil
	}

	val := time.Duration(gracePeriod) * time.Millisecond
	return &val
}
