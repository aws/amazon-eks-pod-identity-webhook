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
	"reflect"
	"testing"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache"
	"k8s.io/api/admission/v1beta1"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var rawPodWithoutVolume = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos"
	  }
	],
	"serviceAccountName": "default"
  }
}
`)

var rawWindowsBetaPodWithoutVolume = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos"
	  }
	],
	"serviceAccountName": "default",
	"nodeSelector": {
		"beta.kubernetes.io/arch": "amd64",
		"beta.kubernetes.io/os": "windows"
	}
  }
}
`)

var rawWindowsPodWithoutVolume = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos"
	  }
	],
	"serviceAccountName": "default",
	"nodeSelector": {
		"kubernetes.io/arch": "amd64",
		"kubernetes.io/os": "windows"
	}
  }
}
`)

var rawPodWithVolume = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos"
	  }
	],
	"serviceAccountName": "default",
	"volumes": [
	  {
	    "name": "my-volume"
	  }
	]
  }
}
`)

var rawPodWithInitContainer = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos"
	  }
	],
	"initContainers": [
	  {
		"image": "amazonlinux",
		"name": "initcontainer"
	  }
	],
	"serviceAccountName": "default"
  }
}
`)

var rawPodWithIAMTokenVolume = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos"
	  }
	],
	"serviceAccountName": "default",
	"volumes": [
	  {
	    "name": "aws-iam-token"
	  }
	]
  }
}
`)

var rawPodWithIAMTokenVolumeAndVolumeMount = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos",
		"env": [
			{
				"name": "AWS_ROLE_ARN",
				"value": "arn:aws:iam::111122223333:role/s3-reader"
			},
			{
				"name": "AWS_WEB_IDENTITY_TOKEN_FILE",
				"value": "/var/run/secrets/eks.amazonaws.com/serviceaccount/token"
			}
		],
		"volumeMounts": [
			{
				"mountPath": "/var/run/secrets/eks.amazonaws.com/serviceaccount",
				"name": "aws-iam-token",
				"readOnly": true
			}
		]
	  }
	],
	"serviceAccountName": "default",
	"volumes": [
	  {
	    "name": "aws-iam-token"
	  }
	]
  }
}
`)

var rawWindowsBetaPodWithVolume = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos"
	  }
	],
	"serviceAccountName": "default",
	"nodeSelector": {
		"beta.kubernetes.io/arch": "amd64",
		"beta.kubernetes.io/os": "windows"
	},
	"volumes": [
	  {
		"name": "my-volume"
	  }
	]
  }
}
`)

var rawWindowsPodWithVolume = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos"
	  }
	],
	"serviceAccountName": "default",
	"nodeSelector": {
		"kubernetes.io/arch": "amd64",
		"kubernetes.io/os": "windows"
	},
	"volumes": [
	  {
		"name": "my-volume"
	  }
	]
  }
}
`)

var rawPodWithoutRegion = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos"
	  }
	],
	"serviceAccountName": "default"
  }
}
`)

var rawPodWithAWSRegion = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos",
		"env": [
		  {"name":"AWS_REGION","value":"paris"}
		]
	  }
	],
	"serviceAccountName": "default"
  }
}
`)

var rawPodWithAWSDefaultRegion = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos",
		"env": [
		  {"name":"AWS_DEFAULT_REGION","value":"paris"}
		]
	  }
	],
	"serviceAccountName": "default"
  }
}
`)

var rawPodWithoutFSGroup = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos"
	  }
	],
	"serviceAccountName": "fsgroup"
  }
}
`)

var rawPodWithFSGroup = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos"
	  }
	],
    "securityContext": {
      "fsGroup": 54321
    },
	"serviceAccountName": "fsgroup"
  }
}
`)

var rawPodWithFSGroupAnnotation = []byte(`
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
    "annotations": {
      "eks.amazonaws.com/fs-group": "33333"
    },
	"name": "balajilovesoreos",
	"uid": "be8695c4-4ad0-4038-8786-c508853aa255"
  },
  "spec": {
	"containers": [
	  {
		"image": "amazonlinux",
		"name": "balajilovesoreos"
	  }
	],
    "securityContext": {
      "runAsUser": 2000
    },
	"serviceAccountName": "fsgroup"
  }
}
`)

func getValidReview(pod []byte) *v1beta1.AdmissionReview {
	return &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			UID: "918ef1dc-928f-4525-99ef-988389f263c3",
			Kind: metav1.GroupVersionKind{
				Version: "v1",
				Kind:    "Pod",
			},
			Namespace: "default",
			Operation: "CREATE",
			UserInfo: authenticationv1.UserInfo{
				Username: "kubernetes-admin",
				UID:      "heptio-authenticator-aws:111122223333:AROAR2TG44V5CLZCFPOQZ",
				Groups:   []string{"system:authenticated", "system:masters"},
			},
			Object: runtime.RawExtension{
				Raw: pod,
			},
			DryRun: nil,
		},
		Response: nil,
	}
}

var validPatchIfNoVolumesPresent = []byte(`[{"op":"add","path":"/spec/volumes","value":[{"name":"aws-iam-token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","expirationSeconds":86400,"path":"token"}}]}}]},{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]}]`)
var validPatchIfVolumesPresent = []byte(`[{"op":"add","path":"/spec/volumes/0","value":{"name":"aws-iam-token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","expirationSeconds":86400,"path":"token"}}]}}},{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]}]`)

var validPatchIfIAMTokenVolumePresent = []byte(`[{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_DEFAULT_REGION","value":"seattle"},{"name":"AWS_REGION","value":"seattle"},{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]}]`)
var validPatchIfIAMTokenVolumeAndVolumeMountPresent = []byte(`[{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"},{"name":"AWS_DEFAULT_REGION","value":"seattle"},{"name":"AWS_REGION","value":"seattle"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]}]`)

var validPatchIfWindowsNoVolumesPresent = []byte(`[{"op":"add","path":"/spec/volumes","value":[{"name":"aws-iam-token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","expirationSeconds":86400,"path":"token"}}]}}]},{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"C:\\var\\run\\secrets\\eks.amazonaws.com\\serviceaccount\\token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]}]`)
var validPatchIfWindowsVolumesPresent = []byte(`[{"op":"add","path":"/spec/volumes/0","value":{"name":"aws-iam-token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","expirationSeconds":86400,"path":"token"}}]}}},{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"C:\\var\\run\\secrets\\eks.amazonaws.com\\serviceaccount\\token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]}]`)

var validPatchIfNoRegionPresent = []byte(`[{"op":"add","path":"/spec/volumes","value":[{"name":"aws-iam-token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","expirationSeconds":86400,"path":"token"}}]}}]},{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_DEFAULT_REGION","value":"seattle"},{"name":"AWS_REGION","value":"seattle"},{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]}]`)
var validPatchIfRegionPresent = []byte(`[{"op":"add","path":"/spec/volumes","value":[{"name":"aws-iam-token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","expirationSeconds":86400,"path":"token"}}]}}]},{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_REGION","value":"paris"},{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]}]`)
var validPatchIfDefaultRegionPresent = []byte(`[{"op":"add","path":"/spec/volumes","value":[{"name":"aws-iam-token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","expirationSeconds":86400,"path":"token"}}]}}]},{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_DEFAULT_REGION","value":"paris"},{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]}]`)
var validPatchIfInitContainerPresent = []byte(`[{"op":"add","path":"/spec/volumes","value":[{"name":"aws-iam-token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","expirationSeconds":86400,"path":"token"}}]}}]},{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_DEFAULT_REGION","value":"seattle"},{"name":"AWS_REGION","value":"seattle"},{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]},{"op":"add","path":"/spec/initContainers","value":[{"name":"initcontainer","image":"amazonlinux","env":[{"name":"AWS_DEFAULT_REGION","value":"seattle"},{"name":"AWS_REGION","value":"seattle"},{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]}]`)

var validPatchIfNoFSGroupPresent = []byte(`[{"op":"add","path":"/spec/volumes","value":[{"name":"aws-iam-token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","expirationSeconds":86400,"path":"token"}}]}}]},{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]},{"op":"add","path":"/spec/securityContext","value":{"fsGroup":12345}}]`)
var validPatchIfFSGroupPresent = []byte(`[{"op":"add","path":"/spec/volumes","value":[{"name":"aws-iam-token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","expirationSeconds":86400,"path":"token"}}]}}]},{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]}]`)
var validPatchIfFSGroupAnnotationPresent = []byte(`[{"op":"add","path":"/spec/volumes","value":[{"name":"aws-iam-token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","expirationSeconds":86400,"path":"token"}}]}}]},{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]},{"op":"add","path":"/spec/securityContext","value":{"runAsUser":2000,"fsGroup":33333}}]`)

var jsonPatchType = v1beta1.PatchType("JSONPatch")

var validResponseIfNoVolumesPresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfNoVolumesPresent,
	PatchType: &jsonPatchType,
}

var validResponseIfVolumesPresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfVolumesPresent,
	PatchType: &jsonPatchType,
}

var validResponseIfIAMTokenVolumePresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfIAMTokenVolumePresent,
	PatchType: &jsonPatchType,
}

var validResponseIfIAMTokenVolumeAndVolumeMountPresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfIAMTokenVolumeAndVolumeMountPresent,
	PatchType: &jsonPatchType,
}

var validResponseIfWindowsNoVolumesPresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfWindowsNoVolumesPresent,
	PatchType: &jsonPatchType,
}

var validResponseIfWindowsVolumesPresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfWindowsVolumesPresent,
	PatchType: &jsonPatchType,
}

var validResponseIfNoRegionPresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfNoRegionPresent,
	PatchType: &jsonPatchType,
}

var validResponseIfRegionPresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfRegionPresent,
	PatchType: &jsonPatchType,
}

var validResponseIfDefaultRegionPresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfDefaultRegionPresent,
	PatchType: &jsonPatchType,
}

var validResponseIfInitContainerPresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfInitContainerPresent,
	PatchType: &jsonPatchType,
}

var validResponseIfNoFSGroupPresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfNoFSGroupPresent,
	PatchType: &jsonPatchType,
}

var validResponseIfFSGroupPresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfFSGroupPresent,
	PatchType: &jsonPatchType,
}

var validResponseIfFSGroupAnnotationPresent = &v1beta1.AdmissionResponse{
	UID:       "",
	Allowed:   true,
	Patch:     validPatchIfFSGroupAnnotationPresent,
	PatchType: &jsonPatchType,
}

func TestSecretStore(t *testing.T) {
	testServiceAccount := &v1.ServiceAccount{}
	testServiceAccount.Name = "default"
	testServiceAccount.Namespace = "default"
	testServiceAccount.Annotations = map[string]string{
		"eks.amazonaws.com/role-arn": "arn:aws:iam::111122223333:role/s3-reader",
	}

	cases := []struct {
		caseName string
		modifier *Modifier
		input    *v1beta1.AdmissionReview
		response *v1beta1.AdmissionResponse
	}{
		{
			"nilBody",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount))),
			nil,
			&v1beta1.AdmissionResponse{Result: &metav1.Status{Message: "bad content"}},
		},
		{
			"NoRequest",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount))),
			&v1beta1.AdmissionReview{Request: nil},
			&v1beta1.AdmissionResponse{Result: &metav1.Status{Message: "bad content"}},
		},
		{
			"ValidRequestSuccessWithoutVolumes",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount))),
			getValidReview(rawPodWithoutVolume),
			validResponseIfNoVolumesPresent,
		},
		{
			"ValidRequestSuccessWindowsWithoutVolumes",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount))),
			getValidReview(rawWindowsBetaPodWithoutVolume),
			validResponseIfWindowsNoVolumesPresent,
		},
		{
			"ValidRequestSuccessWindowsBetaWithoutVolumes",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount))),
			getValidReview(rawWindowsBetaPodWithoutVolume),
			validResponseIfWindowsNoVolumesPresent,
		},
		{
			"ValidRequestSuccessWithVolumes",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount))),
			getValidReview(rawPodWithVolume),
			validResponseIfVolumesPresent,
		},
		{
			"ValidRequestSuccessWindowsWithVolumes",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount))),
			getValidReview(rawWindowsPodWithVolume),
			validResponseIfWindowsVolumesPresent,
		},
		{
			"ValidRequestSuccessWindowsBetaWithVolumes",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount))),
			getValidReview(rawWindowsBetaPodWithVolume),
			validResponseIfWindowsVolumesPresent,
		},
	}

	for _, c := range cases {
		t.Run(c.caseName, func(t *testing.T) {
			response := c.modifier.MutatePod(c.input)

			if !reflect.DeepEqual(response, c.response) {
				got, _ := json.MarshalIndent(response, "", "  ")
				want, _ := json.MarshalIndent(c.response, "", "  ")
				t.Errorf("Unexpected response. Got \n%s\n wanted \n%s", string(got), string(want))
			}

		})
	}
}

func TestEnvUpdate(t *testing.T) {
	testServiceAccount := &v1.ServiceAccount{}
	testServiceAccount.Name = "default"
	testServiceAccount.Namespace = "default"
	testServiceAccount.Annotations = map[string]string{
		"eks.amazonaws.com/role-arn": "arn:aws:iam::111122223333:role/s3-reader",
	}

	cases := []struct {
		caseName string
		modifier *Modifier
		input    *v1beta1.AdmissionReview
		response *v1beta1.AdmissionResponse
	}{
		{
			"ValidRequestSuccessWithoutRegion",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount)), WithRegion("seattle")),
			getValidReview(rawPodWithoutVolume),
			validResponseIfNoRegionPresent,
		},
		{
			"ValidRequestSuccessWithRegion",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount)), WithRegion("seattle")),
			getValidReview(rawPodWithAWSRegion),
			validResponseIfRegionPresent,
		},
		{
			"ValidRequestSuccessWithDefaultRegion",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount)), WithRegion("seattle")),
			getValidReview(rawPodWithAWSDefaultRegion),
			validResponseIfDefaultRegionPresent,
		},
		{
			"ValidRequestSuccessWithIAMTokenVolumePresent",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount)), WithRegion("seattle")),
			getValidReview(rawPodWithIAMTokenVolume),
			validResponseIfIAMTokenVolumePresent,
		},
		{
			"ValidRequestSuccessWithIAMTokenVolumeAndVolumeMountPresent",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount)), WithRegion("seattle")),
			getValidReview(rawPodWithIAMTokenVolumeAndVolumeMount),
			validResponseIfIAMTokenVolumeAndVolumeMountPresent,
		},
		{
			"ValidRequestSuccessWithInitContainer",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount)), WithRegion("seattle")),
			getValidReview(rawPodWithInitContainer),
			validResponseIfInitContainerPresent,
		},
	}

	for _, c := range cases {
		t.Run(c.caseName, func(t *testing.T) {
			response := c.modifier.MutatePod(c.input)

			if !reflect.DeepEqual(response, c.response) {
				got, _ := json.MarshalIndent(response, "", "  ")
				want, _ := json.MarshalIndent(c.response, "", "  ")
				t.Errorf("Unexpected response. Got \n%s\n wanted \n%s", string(got), string(want))
			}

		})
	}
}

func TestFSGroupUpdate(t *testing.T) {
	testServiceAccount := &v1.ServiceAccount{}
	testServiceAccount.Name = "fsgroup"
	testServiceAccount.Namespace = "default"
	testServiceAccount.Annotations = map[string]string{
		"eks.amazonaws.com/role-arn": "arn:aws:iam::111122223333:role/s3-reader",
		"eks.amazonaws.com/fs-group": "12345",
	}

	cases := []struct {
		caseName string
		modifier *Modifier
		input    *v1beta1.AdmissionReview
		response *v1beta1.AdmissionResponse
	}{
		{
			"ValidRequestSuccessWithoutFSGroup",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount))),
			getValidReview(rawPodWithoutFSGroup),
			validResponseIfNoFSGroupPresent,
		},
		{
			"ValidRequestSuccessWithFSGroup",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount))),
			getValidReview(rawPodWithFSGroup),
			validResponseIfFSGroupPresent,
		},
		{
			"ValidRequestSuccessWithFSGroupAnnotation",
			NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount))),
			getValidReview(rawPodWithFSGroupAnnotation),
			validResponseIfFSGroupAnnotationPresent,
		},
	}

	for _, c := range cases {
		t.Run(c.caseName, func(t *testing.T) {
			response := c.modifier.MutatePod(c.input)

			if !reflect.DeepEqual(response, c.response) {
				got, _ := json.MarshalIndent(response, "", "  ")
				want, _ := json.MarshalIndent(c.response, "", "  ")
				t.Errorf("Unexpected response. Got \n%s\n wanted \n%s", string(got), string(want))
			}

		})
	}
}
