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

package main

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	goflag "flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache"
	cachedebug "github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache/debug"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cert"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/handler"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	flag "github.com/spf13/pflag"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var webhookVersion = "v0.1.0"

func main() {
	port := flag.Int("port", 443, "Port to listen on")
	metricsPort := flag.Int("metrics-port", 9999, "Port to listen on for metrics and healthz (http)")

	// TODO Group in help text in-cluster/out-of-cluster/business logic flags
	// out-of-cluster kubeconfig / TLS options
	// Check out https://godoc.org/github.com/spf13/pflag#FlagSet.FlagUsagesWrapped
	// and use pflag.Flag.Annotations
	kubeconfig := flag.String("kubeconfig", "", "(out-of-cluster) Absolute path to the API server kubeconfig file")
	apiURL := flag.String("kube-api", "", "(out-of-cluster) The url to the API server")
	tlsKeyFile := flag.String("tls-key", "/etc/webhook/certs/tls.key", "(out-of-cluster) TLS key file path")
	tlsCertFile := flag.String("tls-cert", "/etc/webhook/certs/tls.crt", "(out-of-cluster) TLS certificate file path")

	// in-cluster TLS options
	inCluster := flag.Bool("in-cluster", true, "Use in-cluster authentication and certificate request API")
	serviceName := flag.String("service-name", "pod-identity-webhook", "(in-cluster) The service name fronting this webhook")
	namespaceName := flag.String("namespace", "eks", "(in-cluster) The namespace name this webhook and the tls secret resides in")
	tlsSecret := flag.String("tls-secret", "pod-identity-webhook", "(in-cluster) The secret name for storing the TLS serving cert")

	// annotation/volume configurations
	annotationPrefix := flag.String("annotation-prefix", "eks.amazonaws.com", "The Service Account annotation to look for")
	audience := flag.String("token-audience", "sts.amazonaws.com", "The default audience for tokens. Can be overridden by annotation")
	mountPath := flag.String("token-mount-path", "/var/run/secrets/eks.amazonaws.com/serviceaccount", "The path to mount tokens")
	tokenExpiration := flag.Int64("token-expiration", pkg.DefaultTokenExpiration, "The token expiration")
	region := flag.String("aws-default-region", "", "If set, AWS_DEFAULT_REGION and AWS_REGION will be set to this value in mutated containers")
	regionalSTS := flag.Bool("sts-regional-endpoint", false, "Whether to inject the AWS_STS_REGIONAL_ENDPOINTS=regional env var in mutated pods. Defaults to `false`.")

	version := flag.Bool("version", false, "Display the version and exit")

	debug := flag.Bool("enable-debugging-handlers", false, "Enable debugging handlers. Currently /debug/alpha/cache is supported")

	klog.InitFlags(goflag.CommandLine)
	// Add klog CommandLine flags to pflag CommandLine
	goflag.CommandLine.VisitAll(func(f *goflag.Flag) {
		flag.CommandLine.AddFlag(flag.PFlagFromGoFlag(f))
	})
	flag.Parse()
	// trick goflag.CommandLine into thinking it was called.
	// klog complains if its not been parsed
	_ = goflag.CommandLine.Parse([]string{})

	if *version {
		fmt.Println(webhookVersion)
		os.Exit(0)
	}

	config, err := clientcmd.BuildConfigFromFlags(*apiURL, *kubeconfig)
	if err != nil {
		klog.Fatalf("Error creating config: %v", err.Error())
	}

	config.QPS = 50
	config.Burst = 50

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error creating clientset: %v", err.Error())
	}
	informerFactory := informers.NewSharedInformerFactory(clientset, 60*time.Second)
	informer := informerFactory.Core().V1().ServiceAccounts()

	*tokenExpiration = pkg.ValidateMinTokenExpiration(*tokenExpiration)
	saCache := cache.New(
		*audience,
		*annotationPrefix,
		*regionalSTS,
		*tokenExpiration,
		informer,
	)
	stop := make(chan struct{})
	informerFactory.Start(stop)
	saCache.Start(stop)
	defer close(stop)

	mod := handler.NewModifier(
		handler.WithAnnotationDomain(*annotationPrefix),
		handler.WithMountPath(*mountPath),
		handler.WithServiceAccountCache(saCache),
		handler.WithRegion(*region),
	)

	addr := fmt.Sprintf(":%d", *port)
	metricsAddr := fmt.Sprintf(":%d", *metricsPort)
	mux := http.NewServeMux()

	baseHandler := handler.Apply(
		http.HandlerFunc(mod.Handle),
		handler.InstrumentRoute(),
		handler.Logging(),
	)
	mux.Handle("/mutate", baseHandler)

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok")
	})

	// Register debug endpoint only if flag is enabled
	if *debug {
		debugger := cachedebug.Dumper{
			Cache: saCache,
		}
		// Reuse metrics port to avoid exposing a new port
		metricsMux.HandleFunc("/debug/alpha/cache", debugger.Handle)
		// Expose other debug paths
	}

	// setup signal handler to be passed to certwatcher and http server
	signalHandlerCtx := signals.SetupSignalHandler()
	tlsConfig := &tls.Config{}

	if *inCluster {
		csr := &x509.CertificateRequest{
			Subject: pkix.Name{CommonName: fmt.Sprintf("%s.%s.svc", *serviceName, *namespaceName)},
			DNSNames: []string{
				fmt.Sprintf("%s", *serviceName),
				fmt.Sprintf("%s.%s", *serviceName, *namespaceName),
				fmt.Sprintf("%s.%s.svc", *serviceName, *namespaceName),
				fmt.Sprintf("%s.%s.svc.cluster.local", *serviceName, *namespaceName),
			},
			/*
				// TODO: SANIPs for service IP, but not pod IP
				//IPAddresses: nil,
			*/
		}

		certManager, err := cert.NewServerCertificateManager(
			clientset,
			*namespaceName,
			*tlsSecret,
			csr,
		)
		if err != nil {
			klog.Fatalf("failed to initialize certificate manager: %v", err)
		}
		certManager.Start()
		defer certManager.Stop()

		tlsConfig.GetCertificate = func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
			certificate := certManager.Current()
			if certificate == nil {
				return nil, fmt.Errorf("no serving certificate available for the webhook, is the CSR approved?")
			}
			return certificate, nil
		}
	} else {
		watcher, err := certwatcher.New(*tlsCertFile, *tlsKeyFile)
		if err != nil {
			klog.Fatalf("Error initializing certwatcher: %q", err)
		}

		go func() {
			if err := watcher.Start(signalHandlerCtx); err != nil {
				klog.Fatalf("Error starting certwatcher: %q", err)
			}
		}()

		tlsConfig.GetCertificate = watcher.GetCertificate
	}

	klog.Info("Creating server")
	server := &http.Server{
		Addr:      addr,
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	handler.ShutdownFromContext(signalHandlerCtx, server, time.Duration(10)*time.Second)

	metricsServer := &http.Server{
		Addr:    metricsAddr,
		Handler: metricsMux,
	}

	go func() {
		klog.Infof("Listening on %s", addr)
		if err := server.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
			klog.Fatalf("Error listening: %q", err)
		}
	}()

	klog.Infof("Listening on %s for metrics and healthz", metricsAddr)
	if err := metricsServer.ListenAndServe(); err != http.ErrServerClosed {
		klog.Fatalf("Error listening: %q", err)
	}
	klog.Info("Graceflully closed")
}
