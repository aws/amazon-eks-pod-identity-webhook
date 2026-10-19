/*
  Copyright 2026 Amazon.com, Inc. or its affiliates. All Rights Reserved.

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

package handler

import (
	"encoding/json"
	"testing"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/containercredentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddWorkloadIdentityToContainer(t *testing.T) {
	socketEnv := corev1.EnvVar{
		Name:  pkg.SpiffeEnvVarEndpointSocket,
		Value: "unix://" + pkg.WorkloadIdentitySocketPath,
	}
	socketMount := corev1.VolumeMount{
		Name:      pkg.WorkloadIdentitySocketVolumeName,
		MountPath: pkg.WorkloadIdentitySocketMountPath,
		ReadOnly:  true,
	}

	testCases := []struct {
		name        string
		enabled     bool
		container   corev1.Container
		wantChanged bool
		want        corev1.Container
	}{
		{
			name: "disabled leaves the container unchanged",
			container: corev1.Container{
				Env:          []corev1.EnvVar{{Name: "EXISTING_ENV", Value: "value"}},
				VolumeMounts: []corev1.VolumeMount{{Name: "existing-volume", MountPath: "/existing"}},
			},
			want: corev1.Container{
				Env:          []corev1.EnvVar{{Name: "EXISTING_ENV", Value: "value"}},
				VolumeMounts: []corev1.VolumeMount{{Name: "existing-volume", MountPath: "/existing"}},
			},
		},
		{
			name:        "enabled adds the socket environment variable and mount",
			enabled:     true,
			wantChanged: true,
			want: corev1.Container{
				Env:          []corev1.EnvVar{socketEnv},
				VolumeMounts: []corev1.VolumeMount{socketMount},
			},
		},
		{
			name:    "preserves a user-defined socket environment variable",
			enabled: true,
			container: corev1.Container{
				Env: []corev1.EnvVar{{
					Name:  pkg.SpiffeEnvVarEndpointSocket,
					Value: "unix:///custom/agent.sock",
				}},
			},
			wantChanged: true,
			want: corev1.Container{
				Env: []corev1.EnvVar{{
					Name:  pkg.SpiffeEnvVarEndpointSocket,
					Value: "unix:///custom/agent.sock",
				}},
				VolumeMounts: []corev1.VolumeMount{socketMount},
			},
		},
		{
			name:    "preserves a user-defined socket volume mount",
			enabled: true,
			container: corev1.Container{
				VolumeMounts: []corev1.VolumeMount{{
					Name:      pkg.WorkloadIdentitySocketVolumeName,
					MountPath: "/custom/socket",
				}},
			},
			wantChanged: true,
			want: corev1.Container{
				Env: []corev1.EnvVar{socketEnv},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      pkg.WorkloadIdentitySocketVolumeName,
					MountPath: "/custom/socket",
				}},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			changed := addWorkloadIdentityToContainer(&testCase.container, testCase.enabled)

			assert.Equal(t, testCase.wantChanged, changed)
			assert.Equal(t, testCase.want, testCase.container)
		})
	}
}

func TestWorkloadIdentityPodVolumePatch(t *testing.T) {
	operation, ok := workloadIdentityPodVolumePatch(nil, false)
	assert.False(t, ok)
	assert.Empty(t, operation)

	existingVolume := corev1.Volume{Name: "existing-volume"}
	operation, ok = workloadIdentityPodVolumePatch([]corev1.Volume{existingVolume}, true)
	require.True(t, ok)
	assert.Equal(t, "add", operation.Op)
	assert.Equal(t, "/spec/volumes/-", operation.Path)

	volume, ok := operation.Value.(corev1.Volume)
	require.True(t, ok)
	require.NotNil(t, volume.CSI)
	assert.Equal(t, pkg.WorkloadIdentitySocketVolumeName, volume.Name)
	assert.Equal(t, pkg.WorkloadIdentityCSIDriver, volume.CSI.Driver)
	require.NotNil(t, volume.CSI.ReadOnly)
	assert.True(t, *volume.CSI.ReadOnly)

	conflictingVolume := corev1.Volume{
		Name: pkg.WorkloadIdentitySocketVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	operation, ok = workloadIdentityPodVolumePatch([]corev1.Volume{existingVolume, conflictingVolume}, true)
	assert.False(t, ok)
	assert.Empty(t, operation)
}

func TestMutatePodWorkloadIdentityInjection(t *testing.T) {
	const (
		linuxTokenPath   = "/var/run/secrets/pods.eks.amazonaws.com/serviceaccount/eks-pod-identity-token"
		windowsTokenPath = `C:\var\run\secrets\pods.eks.amazonaws.com\serviceaccount\eks-pod-identity-token`
	)

	testCases := []struct {
		name                 string
		workloadIdentity     bool
		nodeSelector         map[string]string
		wantTokenPath        string
		wantWorkloadIdentity bool
	}{
		{
			name:                 "enabled",
			workloadIdentity:     true,
			wantTokenPath:        linuxTokenPath,
			wantWorkloadIdentity: true,
		},
		{
			name:          "disabled",
			wantTokenPath: linuxTokenPath,
		},
		{
			name:             "not yet supported on Windows",
			workloadIdentity: true,
			nodeSelector:     map[string]string{"kubernetes.io/os": "windows"},
			wantTokenPath:    windowsTokenPath,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			modifier := newWorkloadIdentityModifier(t, testCase.workloadIdentity)
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec: corev1.PodSpec{
					ServiceAccountName: "default",
					NodeSelector:       testCase.nodeSelector,
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "amazonlinux",
					}},
				},
			}

			podBytes, err := json.Marshal(pod)
			require.NoError(t, err)
			response := modifier.MutatePod(getValidReview(podBytes))
			require.True(t, response.Allowed)
			require.NotEmpty(t, response.Patch)
			patch := decodePatch(t, response.Patch)

			containers := containersFromPatch(t, patch)
			require.Len(t, containers, 1)
			container := containers[0]
			assert.Contains(t, container.Env, corev1.EnvVar{
				Name:  pkg.AwsEnvVarContainerCredentialsFullUri,
				Value: "http://169.254.170.23/v1/credentials",
			})
			assert.Contains(t, container.Env, corev1.EnvVar{
				Name:  pkg.AwsEnvVarContainerAuthorizationTokenFile,
				Value: testCase.wantTokenPath,
			})
			assert.Contains(t, container.VolumeMounts, corev1.VolumeMount{
				Name:      "eks-pod-identity-token",
				MountPath: "/var/run/secrets/pods.eks.amazonaws.com/serviceaccount",
				ReadOnly:  true,
			})
			assert.NotNil(t, volumeFromPatch(t, patch, "eks-pod-identity-token"))

			socketEnv := corev1.EnvVar{
				Name:  pkg.SpiffeEnvVarEndpointSocket,
				Value: "unix://" + pkg.WorkloadIdentitySocketPath,
			}
			socketMount := corev1.VolumeMount{
				Name:      pkg.WorkloadIdentitySocketVolumeName,
				MountPath: pkg.WorkloadIdentitySocketMountPath,
				ReadOnly:  true,
			}
			socketVolume := volumeFromPatch(t, patch, pkg.WorkloadIdentitySocketVolumeName)

			if testCase.wantWorkloadIdentity {
				assert.Contains(t, container.Env, socketEnv)
				assert.Contains(t, container.VolumeMounts, socketMount)
				require.NotNil(t, socketVolume)
				require.NotNil(t, socketVolume.CSI)
				assert.Equal(t, pkg.WorkloadIdentityCSIDriver, socketVolume.CSI.Driver)
				require.NotNil(t, socketVolume.CSI.ReadOnly)
				assert.True(t, *socketVolume.CSI.ReadOnly)
			} else {
				assert.False(t, containerHasEnv(container, pkg.SpiffeEnvVarEndpointSocket))
				assert.False(t, containerHasVolumeMount(container, pkg.WorkloadIdentitySocketVolumeName))
				assert.Nil(t, socketVolume)
			}
		})
	}
}

func newWorkloadIdentityModifier(t *testing.T, enabled bool) *Modifier {
	t.Helper()

	return NewModifier(
		getAlwaysZeroRand(t),
		WithServiceAccountCache(cache.NewFakeServiceAccountCache()),
		WithContainerCredentialsConfig(&containercredentials.FakeConfig{
			Audience:   "pods.eks.amazonaws.com",
			MountPath:  "/var/run/secrets/pods.eks.amazonaws.com/serviceaccount",
			VolumeName: "eks-pod-identity-token",
			TokenPath:  "eks-pod-identity-token",
			FullUri:    "http://169.254.170.23/v1/credentials",
			Identities: map[containercredentials.Identity]bool{
				{
					Namespace:        "default",
					ServiceAccount:   "default",
					WorkloadIdentity: enabled,
				}: true,
			},
		}),
	)
}

type rawPatchOperation struct {
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

func decodePatch(t *testing.T, patchBytes []byte) []rawPatchOperation {
	t.Helper()

	var patch []rawPatchOperation
	require.NoError(t, json.Unmarshal(patchBytes, &patch))
	return patch
}

func containersFromPatch(t *testing.T, patch []rawPatchOperation) []corev1.Container {
	t.Helper()

	for _, operation := range patch {
		if operation.Path == "/spec/containers" {
			var containers []corev1.Container
			require.NoError(t, json.Unmarshal(operation.Value, &containers))
			return containers
		}
	}

	require.FailNow(t, "container patch not found")
	return nil
}

func volumeFromPatch(t *testing.T, patch []rawPatchOperation, name string) *corev1.Volume {
	t.Helper()

	for _, operation := range patch {
		switch operation.Path {
		case "/spec/volumes":
			var volumes []corev1.Volume
			require.NoError(t, json.Unmarshal(operation.Value, &volumes))
			for _, volume := range volumes {
				if volume.Name == name {
					return &volume
				}
			}
		case "/spec/volumes/0", "/spec/volumes/-":
			var volume corev1.Volume
			require.NoError(t, json.Unmarshal(operation.Value, &volume))
			if volume.Name == name {
				return &volume
			}
		}
	}

	return nil
}

func containerHasEnv(container corev1.Container, name string) bool {
	for _, env := range container.Env {
		if env.Name == name {
			return true
		}
	}

	return false
}

func containerHasVolumeMount(container corev1.Container, name string) bool {
	for _, mount := range container.VolumeMounts {
		if mount.Name == name {
			return true
		}
	}

	return false
}
