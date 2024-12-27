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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

func TestParsePodAnnotations(t *testing.T) {
	podNoAnnotations := `
      apiVersion: v1
      kind: Pod
      metadata:
        name: balajilovesoreos`
	testcases := []struct {
		name string
		pod  string

		expectedContainersToSkip map[string]bool

		fallbackExpiration int64
		expectedExpiration int64

		fallbackSALookupGracePeriod time.Duration
		expectedSALookupGracePeriod time.Duration
	}{
		{
			name: "sidecar-containers",
			pod: `
              apiVersion: v1
              kind: Pod
              metadata:
                name: balajilovesoreos
                annotations:
                  testing.eks.amazonaws.com/skip-containers: "sidecar,foo"
            `,
			expectedContainersToSkip: map[string]bool{"sidecar": true, "foo": true},
		},
		{
			name: "token-expiration",
			pod: `
              apiVersion: v1
              kind: Pod
              metadata:
                name: balajilovesoreos
                annotations:
                  testing.eks.amazonaws.com/token-expiration: "1234"
            `,
			fallbackExpiration: 4567,
			expectedExpiration: 1234,
		},
		{
			name:               "token-expiration fallback",
			pod:                podNoAnnotations,
			fallbackExpiration: 4567,
			expectedExpiration: 4567,
		},
		{
			name: "token-expiration round up to min value",
			pod: `
              apiVersion: v1
              kind: Pod
              metadata:
                name: balajilovesoreos
                annotations:
                  testing.eks.amazonaws.com/token-expiration: "0"
            `,
			fallbackExpiration: 4567,
			expectedExpiration: 600,
		},
		{
			name: "service-account-lookup-grace-period",
			pod: `
              apiVersion: v1
              kind: Pod
              metadata:
                name: balajilovesoreos
                annotations:
                  testing.eks.amazonaws.com/service-account-lookup-grace-period: "250"
            `,
			fallbackSALookupGracePeriod: time.Duration(0),
			expectedSALookupGracePeriod: time.Duration(250 * time.Millisecond),
		},
		{
			name:                        "service-account-lookup-grace-period fallback",
			pod:                         podNoAnnotations,
			fallbackSALookupGracePeriod: time.Duration(100 * time.Millisecond),
			expectedSALookupGracePeriod: time.Duration(100 * time.Millisecond),
		},
		{
			name: "service-account-lookup-grace-period override with 0 to disable",
			pod: `
              apiVersion: v1
              kind: Pod
              metadata:
                name: balajilovesoreos
                annotations:
                  testing.eks.amazonaws.com/service-account-lookup-grace-period: "0"
            `,
			fallbackSALookupGracePeriod: time.Duration(250 * time.Millisecond),
			expectedSALookupGracePeriod: time.Duration(0),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var pod *corev1.Pod

			err := yaml.Unmarshal([]byte(tc.pod), &pod)
			assert.NoError(t, err)

			actual := ParsePodAnnotations(pod, "testing.eks.amazonaws.com")
			assert.Equal(t, tc.expectedContainersToSkip, actual.GetContainersToSkip())
			assert.Equal(t, tc.expectedExpiration, actual.GetTokenExpiration(tc.fallbackExpiration))
			assert.Equal(t, tc.expectedSALookupGracePeriod, actual.GetSALookupGracePeriod(tc.fallbackSALookupGracePeriod))
		})
	}
}
