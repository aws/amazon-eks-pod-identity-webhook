package cert

/*
  Provides certificate watcher which reads the certificate from the CertificateStore (k8s secret),
  both on startup and on renewal.
  The update must be done externally.

  Suitable for use with OpenShift's "service serving certificate" feature (using the
  service.beta.openshift.io/serving-cert-secret-name annotation to provide certificates to services).
*/

import (
	"crypto/tls"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/certificate"
)

type storeCertWatcher struct {
	certificateStore certificate.Store
}

func NewSecretStoreCertWatcher(kubeClient clientset.Interface, namespace, secretName string) (CertWatcher, error) {
	certificateStore := NewSecretCertStore(
		namespace,
		secretName,
		kubeClient,
	)

	sc := &storeCertWatcher{
		certificateStore: certificateStore,
	}

	return newCertWatcher(sc)
}

func (sc *storeCertWatcher) Load() (*tls.Certificate, error) {
	newCert, err := sc.certificateStore.Current()
	if err != nil {
		return nil, err
	}

	return newCert, nil
}
