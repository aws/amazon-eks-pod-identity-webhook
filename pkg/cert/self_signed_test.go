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

package cert

import (
	"bytes"
	"net/url"
	"testing"
)

var expectedKubeconfig = []byte(`clusters:
- cluster:
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNUekNDQVRlZ0F3SUJBZ0lVR0JSUU43akJqemhxSmszeWtSNEp3ZC9QWWJRd0RRWUpLb1pJaHZjTkFRRUwKQlFBd0ZURVRNQkVHQTFVRUF4TUthM1ZpWlhKdVpYUmxjekFlRncweE9UQTJNRFl4TnpJME1EQmFGdzB5TURBMgpNRFV4TnpJME1EQmFNQ014SVRBZkJnTlZCQU1UR0dsaGJTMW1iM0l0Y0c5a2N5NWtaV1poZFd4MExuTjJZekJaCk1CTUdCeXFHU000OUFnRUdDQ3FHU000OUF3RUhBMElBQkR2S1dOdC9vVlVBRERoQVlFS3JlRlZXZlQrT2gzUHUKTHlsNVRCN2l0a3pHN1Jnam1Yd0VXVFZNSkQ3MjJmTVZ1Z3JYNDZMbW85NDcyVTRXVGxGZ0JEV2pWREJTTUE0RwpBMVVkRHdFQi93UUVBd0lGb0RBVEJnTlZIU1VFRERBS0JnZ3JCZ0VGQlFjREFUQU1CZ05WSFJNQkFmOEVBakFBCk1CMEdBMVVkRGdRV0JCUU53TTd0WFBjWllWbVQwNGJLQkY3TFlVeWZrREFOQmdrcWhraUc5dzBCQVFzRkFBT0MKQVFFQUlvcG1OUDRWWC9xM2hqbTRLS0dlOGhUWCtJRXdRZG1JRFQyaG1LODFlMGZySS9Qcml4Vy8zU05VTnNhOAoxT0xLS2g2MFRyZjNTSzZGbjBRRjkyTTVSY093YmxpK1ozSDhKY2ZwaXk4NEcyaDg2UkpYQUFjSGh0RDJpRFRJCmV5THRXZW5sOXV4WkZGQnZ1NzRSVFRsZFBiZFMzbVRKa3pHTC8yOFJndWNKWEh0RTcyaDNlN2l6K2pWWWN5LysKeDB5N3BFSm5kSVIyck5NUnQ3NExDRmR2VFZGakNkb1N5QU0wVGgyYlVtdk11dElhK0lkTWVXU2MwQVVXTHFCZwplYzVqTk9wVVh4bG9iWWxjUG5oSVVjVjRyaW1KYkZ6RzJlR1ozZXcvM1RtZlA2clBqRnczUDBMNGRvZ3dlWU9ICnZoYmIyVG5LZkNrQ29XaWY0dmt3Y1RzYkJBPT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
    server: https://127.0.0.1:1234
  name: webhook
contexts:
- context:
    cluster: webhook
    user: webhook
  name: webhook
current-context: webhook
preferences: {}
users:
- name: webhook
  user: {}
`)

func parseURLOrPanic(u string) *url.URL {
	r, err := url.Parse(u)
	if err != nil {
		panic(err)
	}
	return r
}

func TestConfigManager(t *testing.T) {

	cases := []struct {
		caseName  string
		url       *url.URL
		generator SelfSignedGenerator
		output    []byte
		err       error
	}{
		{
			"Successful generate",
			parseURLOrPanic("https://127.0.0.1:1234"),
			&selfSignedGenerator{
				hostname:  "https://127.0.0.1:1234",
				certBytes: testCert,
				keyBytes:  testKey,
			},
			expectedKubeconfig,
			nil,
		},
	}

	for _, c := range cases {
		t.Run(c.caseName, func(t *testing.T) {
			manager := NewWebhookConfigManager(*c.url, c.generator)
			output, err := manager.GenerateConfig()
			if err != nil && c.err != nil {
				if c.err.Error() != err.Error() {
					t.Errorf("Unexpected error. Got %+v, wanted %+v", err, c.err)
					return
				}
			}
			if err != nil && c.err == nil {
				t.Errorf("Unexpected error. Got %+v", err)
				return
			}
			if err == nil && c.err != nil {
				t.Errorf("Unexpected no error, expected %+v", c.err)
				return
			}
			if !bytes.Equal(output, c.output) {
				t.Errorf("Unexpected content: Got:\n%s\nExpected:\n%s", string(output), string(c.output))
			}

		})
	}
}
