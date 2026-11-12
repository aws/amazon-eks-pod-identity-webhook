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
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

type fakeCSIDriverChecker struct {
	present bool
	known   bool
	calls   int
}

func (f *fakeCSIDriverChecker) Present(name string) (bool, bool) {
	f.calls++
	return f.present, f.known
}

func TestInformerCSIDriverChecker(t *testing.T) {
	testCases := []struct {
		name        string
		objects     []storagev1.CSIDriver
		start       bool
		wantPresent bool
		wantKnown   bool
	}{
		{
			name:  "unknown before the informer has synced",
			start: false,
		},
		{
			name:        "present when the CSIDriver exists",
			objects:     []storagev1.CSIDriver{{ObjectMeta: metav1.ObjectMeta{Name: pkg.WorkloadIdentityCSIDriver}}},
			start:       true,
			wantPresent: true,
			wantKnown:   true,
		},
		{
			name:      "absent when only other CSIDrivers exist",
			objects:   []storagev1.CSIDriver{{ObjectMeta: metav1.ObjectMeta{Name: "ebs.csi.aws.com"}}},
			start:     true,
			wantKnown: true,
		},
		{
			name:      "absent when no CSIDrivers exist",
			start:     true,
			wantKnown: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			for i := range testCase.objects {
				_, err := client.StorageV1().CSIDrivers().Create(context.Background(), &testCase.objects[i], metav1.CreateOptions{})
				require.NoError(t, err)
			}
			factory := informers.NewSharedInformerFactory(client, 0)
			informer := factory.Storage().V1().CSIDrivers()
			checker := NewCSIDriverChecker(informer)

			if testCase.start {
				stop := make(chan struct{})
				defer close(stop)
				factory.Start(stop)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				require.True(t, cache.WaitForCacheSync(ctx.Done(), informer.Informer().HasSynced))
			}

			present, known := checker.Present(pkg.WorkloadIdentityCSIDriver)
			assert.Equal(t, testCase.wantPresent, present)
			assert.Equal(t, testCase.wantKnown, known)
		})
	}
}

func TestMutatePodWorkloadIdentityCSIDriverCheck(t *testing.T) {
	testCases := []struct {
		name              string
		workloadIdentity  bool
		checker           *fakeCSIDriverChecker
		wantRejected      bool
		wantSocket        bool
		wantCheckerCalled bool
	}{
		{
			name:             "no checker keeps injecting the socket",
			workloadIdentity: true,
			wantSocket:       true,
		},
		{
			name:              "driver present injects the socket",
			workloadIdentity:  true,
			checker:           &fakeCSIDriverChecker{present: true, known: true},
			wantSocket:        true,
			wantCheckerCalled: true,
		},
		{
			name:              "driver absent denies the pod",
			workloadIdentity:  true,
			checker:           &fakeCSIDriverChecker{present: false, known: true},
			wantRejected:      true,
			wantCheckerCalled: true,
		},
		{
			name:              "unknown presence keeps injecting the socket",
			workloadIdentity:  true,
			checker:           &fakeCSIDriverChecker{present: false, known: false},
			wantSocket:        true,
			wantCheckerCalled: true,
		},
		{
			name:             "workload identity disabled never consults the checker",
			workloadIdentity: false,
			checker:          &fakeCSIDriverChecker{present: false, known: true},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			modifier := newWorkloadIdentityModifier(t, testCase.workloadIdentity)
			if testCase.checker != nil {
				WithCSIDriverChecker(testCase.checker)(modifier)
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "wi-pod", Namespace: "default"},
				Spec: corev1.PodSpec{
					ServiceAccountName: "default",
					Containers:         []corev1.Container{{Name: "app", Image: "amazonlinux"}},
				},
			}
			podBytes, err := json.Marshal(pod)
			require.NoError(t, err)

			response := modifier.MutatePod(getValidReview(podBytes))
			assert.Empty(t, response.Warnings)
			if testCase.checker != nil {
				assert.Equal(t, testCase.wantCheckerCalled, testCase.checker.calls > 0)
			}

			if testCase.wantRejected {
				// Denied with a 403 that names the driver and the fix, and no patch.
				require.False(t, response.Allowed)
				require.NotNil(t, response.Result)
				assert.Equal(t, int32(http.StatusForbidden), response.Result.Code)
				assert.Equal(t, metav1.StatusReasonForbidden, response.Result.Reason)
				assert.Contains(t, response.Result.Message, pkg.WorkloadIdentityCSIDriver)
				assert.Contains(t, response.Result.Message, "update the EKS Pod Identity Agent add-on")
				assert.Empty(t, response.Patch)
				return
			}

			require.True(t, response.Allowed)
			patch := decodePatch(t, response.Patch)
			containers := containersFromPatch(t, patch)
			require.Len(t, containers, 1)
			container := containers[0]

			// Pod Identity credentials are injected whenever the pod is admitted.
			assert.True(t, containerHasEnv(container, pkg.AwsEnvVarContainerCredentialsFullUri))
			assert.True(t, containerHasEnv(container, pkg.AwsEnvVarContainerAuthorizationTokenFile))
			assert.NotNil(t, volumeFromPatch(t, patch, "eks-pod-identity-token"))

			socketVolume := volumeFromPatch(t, patch, pkg.WorkloadIdentitySocketVolumeName)
			if testCase.wantSocket {
				assert.True(t, containerHasEnv(container, pkg.SpiffeEnvVarEndpointSocket))
				assert.True(t, containerHasVolumeMount(container, pkg.WorkloadIdentitySocketVolumeName))
				assert.NotNil(t, socketVolume)
			} else {
				assert.False(t, containerHasEnv(container, pkg.SpiffeEnvVarEndpointSocket))
				assert.False(t, containerHasVolumeMount(container, pkg.WorkloadIdentitySocketVolumeName))
				assert.Nil(t, socketVolume)
			}
		})
	}
}
