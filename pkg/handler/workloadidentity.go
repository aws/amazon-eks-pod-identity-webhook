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
	"k8s.io/klog/v2"
)

// workloadIdentityRejectedMessage is returned to the API client when a pod's
// association enables workload identity but the cluster cannot serve the
// socket. For a pod created by a controller it shows up in the controller's
// FailedCreate event, for example on the ReplicaSet of a Deployment.
var workloadIdentityRejectedMessage = "workload identity is enabled for this pod's service account, but CSIDriver " +
	pkg.WorkloadIdentityCSIDriver + " is not installed; update the EKS Pod Identity Agent add-on to a version that supports workload identity"

// checkWorkloadIdentity reports whether the pod can be admitted with the
// workload identity socket. It returns a non-empty rejection message when the
// pod must be denied.
//
// Injecting a CSI volume whose driver is not installed leaves the pod stuck
// in ContainerCreating with a generic FailedMount event, and it then gets no
// AWS credentials at all. Starting the pod without the socket instead would
// silently drop the workload identity the association asked for, so the
// app would only fail at runtime. When the workload identity CSIDriver is
// known to be absent, the pod is therefore denied with a message that names
// the fix. When presence is unknown, the socket is injected as before, so a
// setup problem in the webhook never blocks pods.
//
// The check is cluster-wide. A cluster in the middle of an agent rollout has
// the CSIDriver object while some nodes still run an older agent; a pod
// scheduled to such a node can still fail to mount the socket.
func (m *Modifier) checkWorkloadIdentity(pod *corev1.Pod, requested bool) string {
	if !requested || m.csiDriverChecker == nil {
		return ""
	}
	present, known := m.csiDriverChecker.Present(pkg.WorkloadIdentityCSIDriver)
	if !known || present {
		return ""
	}
	klog.Warningf("Rejecting pod with workload identity: CSIDriver %s not found. %s",
		pkg.WorkloadIdentityCSIDriver, logContext(pod.Name, pod.GenerateName, pod.Spec.ServiceAccountName, pod.Namespace))
	workloadIdentityRejectedCounter.WithLabelValues("csi_driver_missing").Inc()
	return workloadIdentityRejectedMessage
}

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
