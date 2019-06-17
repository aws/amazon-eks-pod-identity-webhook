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
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"github.com/pkg/errors"
	"io/ioutil"
	"math/big"
	"net/url"
	"path/filepath"
	"time"

	kubeconfig "k8s.io/client-go/tools/clientcmd/api/v1"
	"k8s.io/client-go/util/cert"
	"sigs.k8s.io/yaml"
)

const (
	tlsKeyName  = "tls.key"
	tlsCertName = "tls.cert"
)

// WebhookConfigManager is a type for getting a APIserver webhook config
type WebhookConfigManager interface {
	// GenerateConfig returns a kubeconfig-formatted file for the API server to consume the webhook
	GenerateConfig() (marshaledConfig []byte, err error)
}

// NewWebhookConfigManager returns a new WebhookConfigManager
func NewWebhookConfigManager(ep url.URL, gen SelfSignedGenerator) WebhookConfigManager {
	return &webhookConfigManager{ep, gen}
}

// Compile time check that webhookConfigManager implements the WebhookConfigManager interface
var _ WebhookConfigManager = &webhookConfigManager{}

type webhookConfigManager struct {
	endpoint  url.URL
	generator SelfSignedGenerator
}

func (m *webhookConfigManager) GenerateConfig() (marshaledConfig []byte, err error) {
	cert, err := m.generator.GetCertificateFn()(nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if len(cert.Certificate) < 1 {
		return nil, errors.New("no cert data found in certificate bytes")
	}
	encodedCert := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Certificate[0],
	})

	cfg := &kubeconfig.Config{
		Clusters: []kubeconfig.NamedCluster{
			kubeconfig.NamedCluster{
				Name: "webhook",
				Cluster: kubeconfig.Cluster{
					Server:                   m.endpoint.String(),
					CertificateAuthorityData: encodedCert,
				},
			},
		},
		AuthInfos: []kubeconfig.NamedAuthInfo{
			kubeconfig.NamedAuthInfo{
				Name:     "webhook",
				AuthInfo: kubeconfig.AuthInfo{},
			},
		},
		Contexts: []kubeconfig.NamedContext{
			kubeconfig.NamedContext{
				Name: "webhook",
				Context: kubeconfig.Context{
					Cluster:  "webhook",
					AuthInfo: "webhook",
				},
			},
		},
		CurrentContext: "webhook",
	}

	return yaml.Marshal(cfg)
}

// SelfSignedGenerator returns a self-signed certificate getting func
type SelfSignedGenerator interface {
	GetCertificateFn() func(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

type selfSignedGenerator struct {
	hostname  string
	certDir   string
	certBytes []byte
	keyBytes  []byte
	lifetime  time.Duration
}

// Compile time check that selfSignedGenerator implements the SelfSignedGenerator interface
var _ SelfSignedGenerator = &selfSignedGenerator{}

// NewSelfSignedGenerator returns a SelfSignedGenerator with a configurable life
func NewSelfSignedGenerator(hostname string, certDir string, lifetime time.Duration) SelfSignedGenerator {
	return &selfSignedGenerator{
		hostname: hostname,
		certDir:  certDir,
		lifetime: lifetime,
	}
}

func getOrCreateCert(certDir, hostname string, lifetime time.Duration) (certBytes, keyBytes []byte, err error) {
	keyPath := filepath.Join(certDir, tlsKeyName)
	certPath := filepath.Join(certDir, tlsCertName)
	if ok, _ := cert.CanReadCertAndKey(certPath, keyPath); ok {
		keyBytes, err = ioutil.ReadFile(keyPath)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
		certBytes, err = ioutil.ReadFile(certPath)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
	} else {
		certBytes, keyBytes, err = selfSignedCertificate(hostname, lifetime)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
		err = cert.WriteCert(keyPath, keyBytes)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
		err = cert.WriteCert(certPath, certBytes)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
	}
	return certBytes, keyBytes, nil
}

func (g *selfSignedGenerator) getCertificate() (*tls.Certificate, error) {
	var err error
	if g.certBytes == nil || g.keyBytes == nil {
		g.certBytes, g.keyBytes, err = getOrCreateCert(g.certDir, g.hostname, g.lifetime)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	cert, err := tls.X509KeyPair(g.certBytes, g.keyBytes)

	if len(cert.Certificate) < 1 {
		return nil, errors.New("no cert data found in certificate bytes")

	}
	certs, err := x509.ParseCertificates(cert.Certificate[0])
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse certificate data")
	}
	cert.Leaf = certs[0]
	return &cert, nil
}

func (g *selfSignedGenerator) GetCertificateFn() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return g.getCertificate()
	}
}

func selfSignedCertificate(hostname string, lifetime time.Duration) ([]byte, []byte, error) {
	// generate a new RSA-2048 keypair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(lifetime)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{CommonName: hostname},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{hostname},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	keyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	certBytes = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	keyBytes = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes})

	return certBytes, keyBytes, nil
}
