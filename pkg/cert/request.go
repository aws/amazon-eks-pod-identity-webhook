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
	"crypto/x509"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	certificates "k8s.io/api/certificates/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/certificate"
)

// NewServerCertificateManager returns a certificate manager that stores TLS keys in Kubernetes Secrets
func NewServerCertificateManager(kubeClient clientset.Interface, namespace, secretName string, csr *x509.CertificateRequest) (certificate.Manager, error) {
	clientsetFn := func(_ *tls.Certificate) (clientset.Interface, error) {
		return kubeClient, nil
	}

	certificateStore := NewSecretCertStore(
		namespace,
		secretName,
		kubeClient,
	)

	var certificateRotation = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Subsystem: "certificate_manager",
			Name:      "server_rotation_seconds",
			Help:      "Histogram of the lifetime of a certificate. The value is the time in seconds the certificate lived before getting rotated",
		},
	)
	prometheus.MustRegister(certificateRotation)

	m, err := certificate.NewManager(&certificate.Config{
		ClientsetFn: clientsetFn,
		Template:    csr,
		Usages: []certificates.KeyUsage{
			// https://tools.ietf.org/html/rfc5280#section-4.2.1.3
			//
			// Digital signature allows the certificate to be used to verify
			// digital signatures used during TLS negotiation.
			certificates.UsageDigitalSignature,
			// KeyEncipherment allows the cert/key pair to be used to encrypt
			// keys, including the symmetric keys negotiated during TLS setup
			// and used for data transfer.
			certificates.UsageKeyEncipherment,
			// ServerAuth allows the cert to be used by a TLS server to
			// authenticate itself to a TLS client.
			certificates.UsageServerAuth,
		},
		// Hard coding this since LegacyUnknownSignerName is no longer available in certificates/v1
		// https://kubernetes.io/docs/reference/access-authn-authz/certificate-signing-requests/#kubernetes-signers.
		SignerName:          "kubernetes.io/legacy-unknown",
		CertificateStore:    certificateStore,
		CertificateRotation: certificateRotation,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize server certificate manager: %v", err)
	}
	return m, nil
}
