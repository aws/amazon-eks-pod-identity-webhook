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

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/certificate"
	"k8s.io/klog"
)

// Compile time check that secretCertStore implements the certificate.Store interface
var _ certificate.Store = &secretCertStore{}

type secretCertStore struct {
	namespace  string
	secretName string
	clientset  clientset.Interface
}

// NewSecretCertStore returns a certificate.Store that keeps TLS secrets in a Kubernetes secret object
func NewSecretCertStore(namespace, secretName string, clientset clientset.Interface) certificate.Store {
	return &secretCertStore{
		namespace:  namespace,
		secretName: secretName,
		clientset:  clientset,
	}
}

// Will attempt to read a secret from the Kubernetes Secret store, format and return it.
// Will error out if the secret isn't set, or the certificate inside is not properly formatted.
func (s *secretCertStore) Current() (*tls.Certificate, error) {
	// Try to get a Certificate from Kube Secret.
	// This will be there if there's been a successful deployment of the webhook before in this cluster.
	secret, err := s.clientset.CoreV1().Secrets(s.namespace).Get(
		s.secretName,
		metav1.GetOptions{},
	)
	// Create an appropriate custom Error for if the secret doesn't exist.
	noKeyErr := certificate.NoCertKeyError(
		fmt.Sprintf("Error: No data found for cert/key files at secret %s/%s",
			s.namespace,
			s.secretName))
	// If we had an error getting the secret, exit.
	if err != nil {
		klog.Errorf("Error fetching secret: %v", err.Error())
		klog.Error("This is expected if this is the initial deploy in this cluster.")
		return nil, &noKeyErr
	}

	badSecretErr := certificate.NoCertKeyError(
		fmt.Sprintf("Error: Data found at secret %s/%s is not structured as expected.",
			s.namespace,
			s.secretName))

	klog.Infof("Fetched secret: %s/%s", s.namespace, s.secretName)
	keyBytes, ok := secret.Data[v1.TLSPrivateKeyKey]
	if !ok {
		return nil, &badSecretErr
	}
	certBytes, ok := secret.Data[v1.TLSCertKey]
	if !ok {
		return nil, &badSecretErr
	}
	return loadX509KeyPairData(certBytes, keyBytes)
}

// Will take a TLS certificate and store it to a Kubernetes Secret.
// This will either create a new certificate, or update the existing Secret.
func (s *secretCertStore) Update(cert, key []byte) (*tls.Certificate, error) {
	var secret *v1.Secret
	var err error
	// Try to get the value of the secret we use to hold the Certificate.
	// If we're running in a new cluster, this should be empty.
	secret, err = s.clientset.CoreV1().Secrets(s.namespace).Get(
		s.secretName,
		metav1.GetOptions{},
	)
	// If we got an error, assume that the Secret just isn't set and set it up.
	if err != nil {
		secret = &v1.Secret{}
		secret.Name = s.secretName
		secret.Namespace = s.namespace
		secret.Data = map[string][]byte{
			v1.TLSCertKey:       cert,
			v1.TLSPrivateKeyKey: key,
		}
		secret.Type = v1.SecretTypeTLS
		_, err = s.clientset.CoreV1().Secrets(s.namespace).Create(secret)
		if err != nil {
			klog.Errorf("Error: Could not create Kubernetes Secret: %v", err.Error())
			return nil, err
		}
		return loadX509KeyPairData(cert, key)
	}
	// If there was already something in the Secret, over-write and update with the new Cert.
	secret.Data = map[string][]byte{
		v1.TLSCertKey:       cert,
		v1.TLSPrivateKeyKey: key,
	}
	_, err = s.clientset.CoreV1().Secrets(s.namespace).Update(secret)
	if err != nil {
		klog.Errorf("Error: Could not update Kubernetes Secret: %v", err.Error())
		return nil, err
	}
	return loadX509KeyPairData(cert, key)
}

func loadX509KeyPairData(cert, key []byte) (*tls.Certificate, error) {
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		klog.Errorf("Error: Could not parse these bytes as X509 Keypair: %v", err.Error())
		return nil, err
	}
	certs, err := x509.ParseCertificates(tlsCert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("Error: Unable to parse certificate data: %v", err)
	}
	tlsCert.Leaf = certs[0]
	return &tlsCert, nil
}
