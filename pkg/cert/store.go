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
	"context"
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

func (s *secretCertStore) Current() (*tls.Certificate, error) {
	secret, err := s.clientset.CoreV1().Secrets(s.namespace).Get(
		context.TODO(),
		s.secretName,
		metav1.GetOptions{},
	)
	noKeyErr := certificate.NoCertKeyError(
		fmt.Sprintf("no cert/key files read at secret %s/%s",
			s.namespace,
			s.secretName))
	if err != nil {
		klog.Errorf("Error fetching secret: %v", err.Error())
		return nil, &noKeyErr
	}
	klog.Infof("Fetched secret: %s/%s", s.namespace, s.secretName)
	keyBytes, ok := secret.Data[v1.TLSPrivateKeyKey]
	if !ok {
		return nil, &noKeyErr
	}
	certBytes, ok := secret.Data[v1.TLSCertKey]
	if !ok {
		return nil, &noKeyErr
	}
	return loadX509KeyPairData(certBytes, keyBytes)
}

func (s *secretCertStore) Update(cert, key []byte) (*tls.Certificate, error) {
	var secret *v1.Secret
	var err error
	secret, err = s.clientset.CoreV1().Secrets(s.namespace).Get(
		context.TODO(),
		s.secretName,
		metav1.GetOptions{},
	)
	if err != nil {
		secret = &v1.Secret{}
		secret.Name = s.secretName
		secret.Namespace = s.namespace
		secret.Data = map[string][]byte{
			v1.TLSCertKey:       cert,
			v1.TLSPrivateKeyKey: key,
		}
		secret.Type = v1.SecretTypeTLS
		_, err = s.clientset.CoreV1().Secrets(s.namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
		if err != nil {
			klog.Errorf("Error creating secret: %v", err.Error())
			return nil, err
		}
		return loadX509KeyPairData(cert, key)
	}
	secret.Data = map[string][]byte{
		v1.TLSCertKey:       cert,
		v1.TLSPrivateKeyKey: key,
	}
	_, err = s.clientset.CoreV1().Secrets(s.namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Error updating secret: %v", err.Error())
		return nil, err
	}
	return loadX509KeyPairData(cert, key)
}

func loadX509KeyPairData(cert, key []byte) (*tls.Certificate, error) {
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		klog.Errorf("Error parsing bytes: %v", err.Error())
		return nil, err
	}
	certs, err := x509.ParseCertificates(tlsCert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("unable to parse certificate data: %v", err)
	}
	tlsCert.Leaf = certs[0]
	return &tlsCert, nil
}
