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

package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/containercredentials"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

var fixtureDir = "./testdata"

const (
	// SkipAnnotation means "don't test this file"
	skipAnnotation = "testing.eks.amazonaws.com/skip"
	// Expected patch output
	expectedPatchAnnotation = "testing.eks.amazonaws.com/expectedPatch"

	// Service Account annotation values
	roleArnSAAnnotation               = "testing.eks.amazonaws.com/serviceAccount/roleArn"
	audienceAnnotation                = "testing.eks.amazonaws.com/serviceAccount/audience"
	saInjectSTSAnnotation             = "testing.eks.amazonaws.com/serviceAccount/sts-regional-endpoints"
	saInjectTokenExpirationAnnotation = "testing.eks.amazonaws.com/serviceAccount/token-expiration"

	// Container credentials annotation values
	containerCredentialsFullURIAnnotation    = "testing.eks.amazonaws.com/containercredentials/uri"
	containerCredentialsAudienceAnnotation   = "testing.eks.amazonaws.com/containercredentials/audience"
	containerCredentialsMountPathAnnotation  = "testing.eks.amazonaws.com/containercredentials/mountPath"
	containerCredentialsVolumeNameAnnotation = "testing.eks.amazonaws.com/containercredentials/volumeName"
	containerCredentialsTokenPathAnnotation  = "testing.eks.amazonaws.com/containercredentials/tokenPath"

	// Handler values
	handlerMountPathAnnotation  = "testing.eks.amazonaws.com/handler/mountPath"
	handlerExpirationAnnotation = "testing.eks.amazonaws.com/handler/expiration"
	handlerRegionAnnotation     = "testing.eks.amazonaws.com/handler/region"
	handlerSTSAnnotation        = "testing.eks.amazonaws.com/handler/injectSTS"
)

// buildModifierFromPod gets values to set up test case environments with as if
// the values were set by service account annotation/flag before the test case.
// Test cases are defined entirely by pod yamls.
func buildModifierFromPod(pod *corev1.Pod) *Modifier {
	var modifierOpts []ModifierOpt

	if path, ok := pod.Annotations[handlerMountPathAnnotation]; ok {
		modifierOpts = append(modifierOpts, WithMountPath(path))
	}

	if region, ok := pod.Annotations[handlerRegionAnnotation]; ok {
		modifierOpts = append(modifierOpts, WithRegion(region))
	}

	modifierOpts = append(modifierOpts, WithServiceAccountCache(buildFakeCacheFromPod(pod)))
	modifierOpts = append(modifierOpts, WithContainerCredentialsConfig(buildFakeConfigFromPod(pod)))

	return NewModifier(modifierOpts...)
}

func buildFakeCacheFromPod(pod *corev1.Pod) *cache.FakeServiceAccountCache {
	testServiceAccount := &corev1.ServiceAccount{}
	testServiceAccount.Name = "default"
	testServiceAccount.Namespace = "default"
	testServiceAccount.Annotations = map[string]string{}

	if role, ok := pod.Annotations[roleArnSAAnnotation]; ok {
		testServiceAccount.Annotations["eks.amazonaws.com/role-arn"] = role
	}

	if aud, ok := pod.Annotations[audienceAnnotation]; ok {
		testServiceAccount.Annotations["eks.amazonaws.com/audience"] = aud
	}

	for _, annotationKey := range []string{saInjectSTSAnnotation, handlerSTSAnnotation} {
		if regionalSTS, ok := pod.Annotations[annotationKey]; ok {
			testServiceAccount.Annotations["eks.amazonaws.com/sts-regional-endpoints"] = regionalSTS
			break
		}
	}

	for _, annotationKey := range []string{saInjectTokenExpirationAnnotation, handlerExpirationAnnotation} {
		if tokenExpiration, ok := pod.Annotations[annotationKey]; ok {
			testServiceAccount.Annotations["eks.amazonaws.com/token-expiration"] = tokenExpiration
			break
		}
	}

	return cache.NewFakeServiceAccountCache(testServiceAccount)
}

func buildFakeConfigFromPod(pod *corev1.Pod) *containercredentials.FakeConfig {
	containerCredentialsFullURI := pod.Annotations[containerCredentialsFullURIAnnotation]
	if containerCredentialsFullURI != "" {
		identity := containercredentials.Identity{
			Namespace:      "default",
			ServiceAccount: "default",
		}
		return &containercredentials.FakeConfig{
			Audience:   pod.Annotations[containerCredentialsAudienceAnnotation],
			MountPath:  pod.Annotations[containerCredentialsMountPathAnnotation],
			VolumeName: pod.Annotations[containerCredentialsVolumeNameAnnotation],
			TokenPath:  pod.Annotations[containerCredentialsTokenPathAnnotation],
			FullUri:    containerCredentialsFullURI,
			Identities: map[containercredentials.Identity]bool{
				identity: true,
			},
		}
	}
	return &containercredentials.FakeConfig{}
}

func TestUpdatePodSpec(t *testing.T) {
	err := filepath.Walk(fixtureDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Errorf("Error while walking test fixtures: %v", err)
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".yaml") || strings.HasSuffix(info.Name(), ".yml") {
			pod, err := parseFile(filepath.Join("./", path))
			if err != nil {
				t.Errorf("Error while parsing file %s: %v", info.Name(), err)
				return err
			}
			if skipStr, ok := pod.Annotations[skipAnnotation]; ok {
				skip, _ := strconv.ParseBool(skipStr)
				if skip {
					return nil
				}
			}

			pod.Namespace = "default"
			pod.Spec.ServiceAccountName = "default"

			t.Run(fmt.Sprintf("Pod %s in file %s", pod.Name, path), func(t *testing.T) {
				modifier := buildModifierFromPod(pod)
				patchConfig := modifier.buildPodPatchConfig(pod)
				patch, _ := modifier.getPodSpecPatch(pod, patchConfig)
				patchBytes, err := json.Marshal(patch)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				expectedPatchStr, ok := pod.Annotations[expectedPatchAnnotation]
				if !ok && (len(patchBytes) == 0 || patchBytes == nil) {
					return
				}

				if bytes.Compare(patchBytes, []byte(expectedPatchStr)) != 0 {
					t.Errorf("Expected patch didn't match: \nGot\n\t%v\nWanted:\n\t%v\n", string(patchBytes), expectedPatchStr)
				}

			})
			return nil
		}
		return nil
	})
	if err != nil {
		t.Errorf("Error while walking test fixtures: %v", err)
	}
}

// Read in the first pod in the file
func parseFile(filename string) (*corev1.Pod, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	pod := &corev1.Pod{}
	err = yaml.Unmarshal(data, pod)
	return pod, err
}
