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
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache"
	"k8s.io/api/admission/v1beta1"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestMutatePod(t *testing.T) {
	testServiceAccount := &v1.ServiceAccount{}
	testServiceAccount.Name = "default"
	testServiceAccount.Namespace = "default"
	testServiceAccount.Annotations = map[string]string{
		"eks.amazonaws.com/role-arn": "arn:aws:iam::111122223333:role/s3-reader",
	}

	modifier := NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount)))
	cases := []struct {
		caseName string
		input    *v1beta1.AdmissionReview
		response *v1beta1.AdmissionResponse
	}{
		{
			"nilBody",
			nil,
			&v1beta1.AdmissionResponse{Result: &metav1.Status{Message: "bad content"}},
		},
		{
			"NoRequest",
			&v1beta1.AdmissionReview{Request: nil},
			&v1beta1.AdmissionResponse{Result: &metav1.Status{Message: "bad content"}},
		},
	}

	for _, c := range cases {
		t.Run(c.caseName, func(t *testing.T) {
			response := modifier.MutatePod(c.input)

			if !reflect.DeepEqual(response, c.response) {
				got, _ := json.MarshalIndent(response, "", "  ")
				want, _ := json.MarshalIndent(c.response, "", "  ")
				t.Errorf("Unexpected response. Got \n%s\n wanted \n%s", string(got), string(want))
			}
		})
	}
}

var jsonPatchType = v1beta1.PatchType("JSONPatch")

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

var validPatchIfNoVolumesPresent = []byte(`[{"op":"add","path":"/spec/volumes","value":[{"name":"aws-iam-token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","expirationSeconds":3600,"path":"token"}}]}}]},{"op":"add","path":"/spec/containers","value":[{"name":"balajilovesoreos","image":"amazonlinux","env":[{"name":"AWS_ROLE_ARN","value":"arn:aws:iam::111122223333:role/s3-reader"},{"name":"AWS_WEB_IDENTITY_TOKEN_FILE","value":"/var/run/secrets/eks.amazonaws.com/serviceaccount/token"}],"resources":{},"volumeMounts":[{"name":"aws-iam-token","readOnly":true,"mountPath":"/var/run/secrets/eks.amazonaws.com/serviceaccount"}]}]}]`)

var validHandlerResponse = &v1beta1.AdmissionResponse{
	UID:       "918ef1dc-928f-4525-99ef-988389f263c3",
	Allowed:   true,
	Patch:     validPatchIfNoVolumesPresent,
	PatchType: &jsonPatchType,
}

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
				UID:      "aws-iam-authenticator:111122223333:AROAR2TG44V5CLZCFPOQZ",
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

func serializeAdmissionReview(t *testing.T, want *v1beta1.AdmissionReview) []byte {
	wantedBytes, err := json.Marshal(want)
	if err != nil {
		t.Errorf("Failed to marshal desired response: %v", err)
		return nil
	}
	return wantedBytes
}

func TestModifierHandler(t *testing.T) {
	testServiceAccount := &corev1.ServiceAccount{}
	testServiceAccount.Name = "default"
	testServiceAccount.Namespace = "default"
	testServiceAccount.Annotations = map[string]string{
		"eks.amazonaws.com/role-arn": "arn:aws:iam::111122223333:role/s3-reader",
		"eks.amazonaws.com/token-expiration": "3600",
	}

	modifier := NewModifier(WithServiceAccountCache(cache.NewFakeServiceAccountCache(testServiceAccount)))

	ts := httptest.NewServer(
		http.HandlerFunc(modifier.Handle),
	)
	defer ts.Close()

	cases := []struct {
		caseName         string
		input            []byte
		inputContentType string
		want             []byte
	}{
		{
			"nilBody",
			nil,
			"application/json",
			serializeAdmissionReview(t, &v1beta1.AdmissionReview{
				Response: &v1beta1.AdmissionResponse{Result: &metav1.Status{Message: "bad content"}},
			}),
		},
		{
			"NoRequest",
			serializeAdmissionReview(t, &v1beta1.AdmissionReview{Request: nil}),
			"application/json",
			serializeAdmissionReview(t, &v1beta1.AdmissionReview{
				Response: &v1beta1.AdmissionResponse{Result: &metav1.Status{Message: "bad content"}},
			}),
		},
		{
			"BadContentType",
			serializeAdmissionReview(t, &v1beta1.AdmissionReview{Request: nil}),
			"application/xml",
			[]byte("Invalid Content-Type, expected `application/json`\n"),
		},
		{
			"InvalidJSON",
			[]byte(`{"request": {"object": "\"metadata\":{\"name\":\"fake\""}`),
			"application/json",
			[]byte(`{"response":{"uid":"","allowed":false,"status":{"metadata":{},"message":"couldn't get version/kind; json parse error: unexpected end of JSON input"}}}`),
		},
		{
			"InvalidPodBytes",
			[]byte(`{"request": {"object": "\"metadata\":{\"name\":\"fake\""}}`),
			"application/json",
			[]byte(`{"response":{"uid":"","allowed":false,"status":{"metadata":{},"message":"json: cannot unmarshal string into Go value of type v1.Pod"}}}`),
		},
		{
			"ValidRequestSuccessWithoutVolumes",
			serializeAdmissionReview(t, getValidReview(rawPodWithoutVolume)),
			"application/json",
			serializeAdmissionReview(t, &v1beta1.AdmissionReview{Response: validHandlerResponse}),
		},
	}

	for _, c := range cases {
		t.Run(c.caseName, func(t *testing.T) {
			var buf io.Reader
			if c.input != nil {
				buf = bytes.NewBuffer(c.input)
			}
			resp, err := http.Post(ts.URL, c.inputContentType, buf)
			if err != nil {
				t.Errorf("Failed to make request: %v", err)
				return
			}
			responseBytes, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Errorf("Failed to read response: %v", err)
				return
			}

			if bytes.Compare(responseBytes, c.want) != 0 {
				t.Errorf("Expected response didn't match: \nGot\n\t\"%v\"\nWanted:\n\t\"%v\"\n",
					string(responseBytes),
					string(c.want),
				)
			}
		})
	}
}
