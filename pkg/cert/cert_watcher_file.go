package cert

/*
  Provides certificate watcher which reads the certificate from a file, both on startup and on renewal.
  The update must be done externally.
*/

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"k8s.io/klog"
)

type fileCertWatcher struct {
	certFile string
	keyFile  string
}

// Monitors cert/keyfile for changes and attempts to reload if updated.
func NewFileCertWatcher(certFile, keyFile string) (CertWatcher, error) {
	fc := &fileCertWatcher{
		certFile: certFile,
		keyFile:  keyFile,
	}

	return newCertWatcher(fc)
}

func (fc *fileCertWatcher) Load() (*tls.Certificate, error) {
	klog.Infof("Loading cert from file %s", fc.certFile)
	newCert, err := tls.LoadX509KeyPair(fc.certFile, fc.keyFile)
	if err != nil {
		return nil, err
	}

	// LoadX509KeyPair does this but does not provide the data, unfortunately.. so some duplicate work..
	certs, err := x509.ParseCertificates(newCert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("unable to parse certificate data: %v", err)
	}
	newCert.Leaf = certs[0]

	klog.Infof("Loaded keypair from %s, %s", fc.certFile, fc.keyFile)

	return &newCert, nil
}
