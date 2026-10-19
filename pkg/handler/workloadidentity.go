/*
  Copyright 2026 Amazon.com, Inc. or its affiliates. All Rights Reserved.

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
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	corev1 "k8s.io/api/core/v1"
)

func addWorkloadIdentityToContainer(container *corev1.Container, enabled bool) bool {
	if !enabled {
		return false
	}

	changed := false
	socketEnv := false
	for _, env := range container.Env {
		if env.Name == pkg.SpiffeEnvVarEndpointSocket {
			socketEnv = true
			break
		}
	}
	// Add the socket environment variable only when it is not already defined on the container.
	if !socketEnv {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  pkg.SpiffeEnvVarEndpointSocket,
			Value: "unix://" + pkg.WorkloadIdentitySocketPath,
		})
		changed = true
	}

	socketMount := false
	for _, mount := range container.VolumeMounts {
		if mount.Name == pkg.WorkloadIdentitySocketVolumeName {
			socketMount = true
			break
		}
	}
	if !socketMount {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      pkg.WorkloadIdentitySocketVolumeName,
			MountPath: pkg.WorkloadIdentitySocketMountPath,
			ReadOnly:  true,
		})
		changed = true
	}

	return changed
}

func workloadIdentityPodVolumePatch(volumes []corev1.Volume, enabled bool) (patchOperation, bool) {
	if !enabled {
		return patchOperation{}, false
	}

	for _, volume := range volumes {
		if volume.Name == pkg.WorkloadIdentitySocketVolumeName {
			return patchOperation{}, false
		}
	}

	readOnly := true
	return patchOperation{
		Op:   "add",
		Path: "/spec/volumes/-",
		Value: corev1.Volume{
			Name: pkg.WorkloadIdentitySocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{
					Driver:   pkg.WorkloadIdentityCSIDriver,
					ReadOnly: &readOnly,
				},
			},
		},
	}, true
}
