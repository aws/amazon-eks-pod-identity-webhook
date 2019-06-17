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
	"crypto/tls"
	"fmt"
	"reflect"
	"testing"

	"k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/certificate"
)

var testKey = []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIOZd8XRkpgel1Rn6UmmDkff38E5Y5orLSJxBLUaGvZDdoAoGCCqGSM49
AwEHoUQDQgAEO8pY23+hVQAMOEBgQqt4VVZ9P46Hc+4vKXlMHuK2TMbtGCOZfARZ
NUwkPvbZ8xW6Ctfjouaj3jvZThZOUWAENQ==
-----END EC PRIVATE KEY-----`)

var testCert = []byte(`-----BEGIN CERTIFICATE-----
MIICTzCCATegAwIBAgIUGBRQN7jBjzhqJk3ykR4Jwd/PYbQwDQYJKoZIhvcNAQEL
BQAwFTETMBEGA1UEAxMKa3ViZXJuZXRlczAeFw0xOTA2MDYxNzI0MDBaFw0yMDA2
MDUxNzI0MDBaMCMxITAfBgNVBAMTGGlhbS1mb3ItcG9kcy5kZWZhdWx0LnN2YzBZ
MBMGByqGSM49AgEGCCqGSM49AwEHA0IABDvKWNt/oVUADDhAYEKreFVWfT+Oh3Pu
Lyl5TB7itkzG7RgjmXwEWTVMJD722fMVugrX46Lmo9472U4WTlFgBDWjVDBSMA4G
A1UdDwEB/wQEAwIFoDATBgNVHSUEDDAKBggrBgEFBQcDATAMBgNVHRMBAf8EAjAA
MB0GA1UdDgQWBBQNwM7tXPcZYVmT04bKBF7LYUyfkDANBgkqhkiG9w0BAQsFAAOC
AQEAIopmNP4VX/q3hjm4KKGe8hTX+IEwQdmIDT2hmK81e0frI/PrixW/3SNUNsa8
1OLKKh60Trf3SK6Fn0QF92M5RcOwbli+Z3H8Jcfpiy84G2h86RJXAAcHhtD2iDTI
eyLtWenl9uxZFFBvu74RTTldPbdS3mTJkzGL/28RgucJXHtE72h3e7iz+jVYcy/+
x0y7pEJndIR2rNMRt74LCFdvTVFjCdoSyAM0Th2bUmvMutIa+IdMeWSc0AUWLqBg
ec5jNOpUXxlobYlcPnhIUcV4rimJbFzG2eGZ3ew/3TmfP6rPjFw3P0L4dogweYOH
vhbb2TnKfCkCoWif4vkwcTsbBA==
-----END CERTIFICATE-----`)

var testUpdateKey = []byte(`-----BEGIN PRIVATE KEY-----
MIIBVwIBADANBgkqhkiG9w0BAQEFAASCAUEwggE9AgEAAkEAyFA2xiawGkA11OuD
ff6NYpUJsKXSW5DO+Jd9CxLd6ZSNWqzzmpPjwoxMyzA5D7odq0UNvtGmK2a0y64h
UQAzLQIDAQABAkEApgXFwCnkn31EoLqqe0z1hhWcuGpXlUjKIkP8gacbgjIDq/Z2
393bZx8g5YC9zK+yUJFAhJREPqhZiMhqygWOiQIhAO6HeHQWsYdfNXX44EiJxcCE
R5l314rO5+aBkKyHuJQ/AiEA1vwt27fuVdeAnW7oXF5W7TLjPrD/Iu94w0siH8MM
LZMCIQDgdks7sz9MjKPaaGFm4X9eMxzNpqEG1r4ThEmIkg94MQIhAKPpwk0z/9QT
a0ydsyw6Aaz4j6rM6LqKO1krf+kXncFhAiEApHJv2ruRkyhvINOhGjU0/vqwueci
4hE4TYZxPVv6K6Y=
-----END PRIVATE KEY-----`)

var testUpdateCert = []byte(`-----BEGIN CERTIFICATE-----
MIIBqzCCAVWgAwIBAgIJAPZS/SPnqjXHMA0GCSqGSIb3DQEBCwUAMDExLzAtBgNV
BAMMJmlhbS1mb3ItcG9kcy5kZWZhdWx0LnN2Yy5jbHVzdGVyLmxvY2FsMB4XDTE5
MDYwNzE3Mzc0N1oXDTIwMDYwNjE3Mzc0N1owMTEvMC0GA1UEAwwmaWFtLWZvci1w
b2RzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwwXDANBgkqhkiG9w0BAQEFAANL
ADBIAkEAyFA2xiawGkA11OuDff6NYpUJsKXSW5DO+Jd9CxLd6ZSNWqzzmpPjwoxM
yzA5D7odq0UNvtGmK2a0y64hUQAzLQIDAQABo1AwTjAdBgNVHQ4EFgQUHWcrM+Zu
3FWa5xpl/Sifq9ActeowHwYDVR0jBBgwFoAUHWcrM+Zu3FWa5xpl/Sifq9Acteow
DAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAANBAJC81pfww8h4B4Fs2ZoO2Kjn
VyO54BamLyRowfDEItc0eBUmrLzdLS+6iF9UskNCWJid5MEycb+Hmt3U5+PSSY4=
-----END CERTIFICATE-----`)

func TestSecretStore(t *testing.T) {
	noKeyError := certificate.NoCertKeyError("no cert/key files read at secret default/iam-for-pods")
	testCertificate, err := loadX509KeyPairData(testCert, testKey)
	if err != nil {
		t.Errorf("Error parsing test key: %v", err.Error())
		return
	}

	testUpdateCertificate, err := loadX509KeyPairData(testUpdateCert, testUpdateKey)
	if err != nil {
		t.Errorf("Error parsing test key: %v", err.Error())
		return
	}

	testSecret := &v1.Secret{
		Data: map[string][]byte{
			v1.TLSCertKey:       testCert,
			v1.TLSPrivateKeyKey: testKey,
		},
		Type: v1.SecretTypeTLS,
	}
	testSecret.Name = "iam-for-pods"
	testSecret.Namespace = "default"

	cases := []struct {
		caseName        string
		clientset       clientset.Interface
		currentErr      error
		namespace       string
		secret          string
		certificate     *tls.Certificate
		updateErr       error
		updateKeyBytes  []byte
		updateCertBytes []byte
		updatedCert     *tls.Certificate
	}{
		{
			"NoSecretUpdateSuccess",
			fakeclientset.NewSimpleClientset(),
			&noKeyError,
			"default",
			"iam-for-pods",
			nil,
			nil,
			testUpdateKey,
			testUpdateCert,
			testUpdateCertificate,
		},
		{
			"SecretExistsErrorUpdating",
			fakeclientset.NewSimpleClientset(testSecret),
			nil,
			"default",
			"iam-for-pods",
			testCertificate,
			fmt.Errorf("tls: failed to find any PEM data in certificate input"),
			[]byte("invalid-key"),
			[]byte("invalid-cert"),
			nil,
		},
		{
			"SecretExistsUpdateSuccess",
			fakeclientset.NewSimpleClientset(testSecret),
			nil,
			"default",
			"iam-for-pods",
			testCertificate,
			nil,
			testUpdateKey,
			testUpdateCert,
			testUpdateCertificate,
		},
	}

	for _, c := range cases {
		t.Run(c.caseName, func(t *testing.T) {
			store := NewSecretCertStore(c.namespace, c.secret, c.clientset)
			currentCert, err := store.Current()
			if err != nil && c.currentErr != nil {
				if c.currentErr.Error() != err.Error() {
					t.Errorf(" Unexpected error. Got %v, wanted %v", err, c.currentErr)
					return
				}
			}
			if err != nil && c.currentErr == nil {
				t.Errorf("Unexpected error. Got %v", err)
				return
			}
			if err == nil && c.currentErr != nil {
				t.Errorf("Unexpected no error, expected %v", c.currentErr)
				return
			}

			if !reflect.DeepEqual(currentCert, c.certificate) {
				t.Errorf("Unexpected certificate. Got %#v wanted %#v", currentCert, c.certificate)
				return
			}

			updatedCert, err := store.Update(c.updateCertBytes, c.updateKeyBytes)
			if err != nil && c.updateErr != nil {
				if c.updateErr.Error() != err.Error() {
					t.Errorf(" Unexpected error. Got '%v', wanted '%v'", err, c.updateErr)
				}
				return
			}
			if err != nil && c.updateErr == nil {
				t.Errorf("Unexpected error. Got %v", err)
				return
			}
			if err == nil && c.updateErr != nil {
				t.Errorf("Unexpected no error, expected %v", c.updateErr)
				return
			}
			if !reflect.DeepEqual(updatedCert, c.updatedCert) {
				t.Errorf("Unexpected certificate. Got %#v wanted %#v", updatedCert, c.updatedCert)
				return
			}

		})
	}
}
