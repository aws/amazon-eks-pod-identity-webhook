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
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

var fixtureDir = "./testdata"

var (
	// SkipAnnotation means "don't test this file"
	skipAnnotation = "testing.eks.amazonaws.com/skip"
	// Expected patch output
	expectedPatchAnnotation = "testing.eks.amazonaws.com/expectedPatch"

	// Service Account annotation values
	roleArnSAAnnotation               = "testing.eks.amazonaws.com/serviceAccount/roleArn"
	audienceAnnotation                = "testing.eks.amazonaws.com/serviceAccount/audience"
	saInjectSTSAnnotation             = "testing.eks.amazonaws.com/serviceAccount/sts-regional-endpoints"
	saInjectTokenExpirationAnnotation = "testing.eks.amazonaws.com/serviceAccount/token-expiration"

	// Handler values
	handlerMountPathAnnotation  = "testing.eks.amazonaws.com/handler/mountPath"
	handlerExpirationAnnotation = "testing.eks.amazonaws.com/handler/expiration"
	handlerRegionAnnotation     = "testing.eks.amazonaws.com/handler/region"
	handlerSTSAnnotation        = "testing.eks.amazonaws.com/handler/injectSTS"
)

// getTestValuesFromPod gets values to set up test case environments with as if
// the values were set by service account annotation/flag before the test case.
// Test cases are defined entirely by pod yamls.
func getTestValuesFromPod(pod corev1.Pod) (*Modifier, int64, bool, error) {
	modifiers := []ModifierOpt{}

	if path, ok := pod.Annotations[handlerMountPathAnnotation]; ok {
		modifiers = append(modifiers, WithMountPath(path))
	}

	tokenExpiration := pkg.DefaultTokenExpiration
	if expStr, ok := pod.Annotations[handlerExpirationAnnotation]; ok {
		expInt, err := strconv.Atoi(expStr)
		if err != nil {
			return nil, 0, false, err
		}
		tokenExpiration = int64(expInt)
	}

	if expStr, ok := pod.Annotations[handlerExpirationAnnotation]; ok {
		expInt, err := strconv.Atoi(expStr)
		if err != nil {
			return nil, 0, false, err
		}
		tokenExpiration = int64(expInt)
	}

	if region, ok := pod.Annotations[handlerRegionAnnotation]; ok {
		modifiers = append(modifiers, WithRegion(region))
	}

	regionalSTS := false
	if stsAnnotation, ok := pod.Annotations[handlerSTSAnnotation]; ok {
		value, err := strconv.ParseBool(stsAnnotation)
		if err != nil {
			return nil, 0, false, err
		}
		regionalSTS = value
	}

	return NewModifier(modifiers...), tokenExpiration, regionalSTS, nil
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

			t.Run(fmt.Sprintf("Pod %s in file %s", pod.Name, path), func(t *testing.T) {
				modifier, tokenExpiration, regionalSTS, err := getTestValuesFromPod(*pod)
				if err != nil {
					t.Errorf("Error creating modifier: %v", err)
				}

				var roleARN string
				if role, ok := pod.Annotations[roleArnSAAnnotation]; ok {
					roleARN = role
				}

				audience := "sts.amazonaws.com"
				if aud, ok := pod.Annotations[audienceAnnotation]; ok {
					audience = aud
				}

				if regionalSTSstr, ok := pod.Annotations[saInjectSTSAnnotation]; ok {
					regionalSTS, err = strconv.ParseBool(regionalSTSstr)
					if err != nil {
						t.Errorf("Error parsing annotation %s: %v", saInjectSTSAnnotation, err)
					}
				}

				if tokenExpirationStr, ok := pod.Annotations[saInjectTokenExpirationAnnotation]; ok {
					tokenExpiration, err = strconv.ParseInt(tokenExpirationStr, 10, 64)
					if err != nil {
						t.Errorf("Error parsing annotation %s: %v", saInjectTokenExpirationAnnotation, err)
					}
				}

				tokenExpiration, containersToSkip := modifier.parsePodAnnotations(pod, tokenExpiration)
				patch, _ := modifier.getPodSpecPatch(pod, roleARN, audience, regionalSTS, tokenExpiration, containersToSkip)
				patchBytes, err := json.Marshal(patch)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				expectedPatchStr, ok := pod.Annotations[expectedPatchAnnotation]
				if !ok && (len(patchBytes) == 0 || patchBytes == nil) {
					return
				}

				if bytes.Compare(patchBytes, []byte(expectedPatchStr)) != 0 {
					t.Errorf("Expected patch didn't match: \nGot\n\t%v\nWanted:\n\t%v\n",
						string(patchBytes),
						expectedPatchStr,
					)
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
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	pod := &corev1.Pod{}
	err = yaml.Unmarshal(data, pod)
	return pod, err
}
