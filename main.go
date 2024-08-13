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
	"strings"
	"time"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache"
	cachedebug "github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache/debug"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cert"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/containercredentials"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/handler"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	flag "github.com/spf13/pflag"
	"k8s.io/client-go/informers"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var webhookVersion = "v0.1.0"

func main() {
	port := flag.Int("port", 443, "Port to listen on")
	metricsPort := flag.Int("metrics-port", 9999, "Port to listen on for metrics (http)")

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
	namespaceName := flag.String("namespace", "eks", "(in-cluster) The namespace name this webhook, the TLS secret, and configmap resides in")
	tlsSecret := flag.String("tls-secret", "pod-identity-webhook", "(in-cluster) The secret name for storing the TLS serving cert")

	// annotation/volume configurations
	annotationPrefix := flag.String("annotation-prefix", "eks.amazonaws.com", "The Service Account annotation to look for")
	audience := flag.String("token-audience", "sts.amazonaws.com", "The default audience for tokens. Can be overridden by annotation")
	mountPath := flag.String("token-mount-path", "/var/run/secrets/eks.amazonaws.com/serviceaccount", "The path to mount tokens")
	tokenExpiration := flag.Int64("token-expiration", pkg.DefaultTokenExpiration, "The token expiration")
	region := flag.String("aws-default-region", "", "If set, AWS_DEFAULT_REGION and AWS_REGION will be set to this value in mutated containers")
	regionalSTS := flag.Bool("sts-regional-endpoint", false, "Whether to inject the AWS_STS_REGIONAL_ENDPOINTS=regional env var in mutated pods. Defaults to `false`.")
	watchConfigMap := flag.Bool("watch-config-map", false, "Enables watching serviceaccounts that are configured through the pod-identity-webhook configmap instead of using annotations")
	composeRoleArn := flag.Bool("compose-role-arn", false, "If true, then the role name and path can be used instead of the fully qualified ARN in the `role-arn` annotation.  In this case, webhook will look up the partition and account ID using instance metadata.  Defaults to `false`.")
	watchContainerCredentialsConfig := flag.String("watch-container-credentials-config", "", "Absolute path to the container credential config file to watch for")
	containerCredentialsAudience := flag.String("container-credentials-audience", "pods.eks.amazonaws.com", "The audience for tokens used by the AWS Container Credentials method")
	containerCredentialsMountPath := flag.String("container-credentials-token-mount-path", "/var/run/secrets/pods.eks.amazonaws.com/serviceaccount", "The path to mount tokens used by the AWS Container Credentials method")
	containerCredentialsVolumeName := flag.String("container-credentials-token-volume-name", "eks-pod-identity-token", "The name of the projected volume containing the injected service account token. This is only used by the AWS Container Credentials method")
	containerCredentialsTokenPath := flag.String("container-credentials-token-path", "eks-pod-identity-token", "The path of the injected service account token. This is only used by the AWS Container Credentials method")
	containerCredentialsFullUri := flag.String("container-credentials-full-uri", "http://169.254.170.23/v1/credentials", "AWS_CONTAINER_CREDENTIALS_FULL_URI will be set to this value in mutated containers")

	version := flag.Bool("version", false, "Display the version and exit")

	debug := flag.Bool("enable-debugging-handlers", false, "Enable debugging handlers. Currently /debug/alpha/cache is supported")

	saLookupGracePeriod := flag.Duration("service-account-lookup-grace-period", 100*time.Millisecond, "The grace period for service account to be available in cache before not mutating a pod. Defaults to 100ms. Set to 0 to deactivate waiting. Carefully use higher values as it may have significant impact on Kubernetes' pod scheduling performance.")

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

	// setup signal handler
	signalHandlerCtx := signals.SetupSignalHandler()

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

	var cmInformer v1.ConfigMapInformer
	var nsInformerFactory informers.SharedInformerFactory
	if *watchConfigMap {
		klog.Infof("Watching ConfigMap pod-identity-webhook in %s namespace", *namespaceName)
		nsInformerFactory = informers.NewSharedInformerFactoryWithOptions(clientset, 60*time.Second, informers.WithNamespace(*namespaceName))
		cmInformer = nsInformerFactory.Core().V1().ConfigMaps()
	}

	saInformer := informerFactory.Core().V1().ServiceAccounts()

	*tokenExpiration = pkg.ValidateMinTokenExpiration(*tokenExpiration)

	var identity ec2metadata.EC2InstanceIdentityDocument
	var composeRoleArnCache cache.ComposeRoleArn
	if *composeRoleArn {
		sess, err := session.NewSession()
		if err != nil {
			klog.Fatalf("Error creating session: %v", err.Error())
		}

		metadataClient := ec2metadata.New(sess)
		identity, err = metadataClient.GetInstanceIdentityDocument()
		if err != nil {
			klog.Fatalf("Error getting instance identity document: %v", err.Error())
		}

		region := identity.Region
		var partition string
		switch {
		case strings.HasPrefix(region, "cn-"):
			partition = "aws-cn"
		case strings.HasPrefix(region, "us-gov-"):
			partition = "aws-us-gov"
		case strings.HasPrefix(region, "us-iso-"):
			partition = "aws-iso"
		case strings.HasPrefix(region, "us-isob-"):
			partition = "aws-iso-b"
		default:
			partition = "aws"
		}

		composeRoleArnCache = cache.ComposeRoleArn{
			Enabled: true,

			AccountID: identity.AccountID,
			Partition: partition,
			Region:    identity.Region,
		}

	}

	saCache := cache.New(
		*audience,
		*annotationPrefix,
		*regionalSTS,
		*tokenExpiration,
		saInformer,
		cmInformer,
		composeRoleArnCache,
	)
	stop := make(chan struct{})
	informerFactory.Start(stop)

	if *watchConfigMap {
		nsInformerFactory.Start(stop)
	}

	saCache.Start(stop)
	defer close(stop)

	containerCredentialsConfig := containercredentials.NewFileConfig(
		*containerCredentialsAudience,
		*containerCredentialsMountPath,
		*containerCredentialsVolumeName,
		*containerCredentialsTokenPath,
		*containerCredentialsFullUri)
	if watchContainerCredentialsConfig != nil && *watchContainerCredentialsConfig != "" {
		klog.Infof("Watching container credentials config file %s", *watchContainerCredentialsConfig)
		err = containerCredentialsConfig.StartWatcher(signalHandlerCtx, *watchContainerCredentialsConfig)
		if err != nil {
			klog.Fatalf("Error starting watcher on file %v: %v", *watchContainerCredentialsConfig, err.Error())
		}
	}

	mod := handler.NewModifier(
		handler.WithAnnotationDomain(*annotationPrefix),
		handler.WithMountPath(*mountPath),
		handler.WithServiceAccountCache(saCache),
		handler.WithContainerCredentialsConfig(containerCredentialsConfig),
		handler.WithRegion(*region),
		handler.WithSALookupGraceTime(*saLookupGracePeriod),
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
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok")
	})

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())

	// Register debug endpoint only if flag is enabled
	if *debug {
		debugger := cachedebug.Dumper{
			Cache: saCache,
		}
		// Reuse metrics port to avoid exposing a new port
		metricsMux.HandleFunc("/debug/alpha/cache", debugger.Handle)
		// Expose other debug paths
	}

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

	handler.ShutdownFromContext(signalHandlerCtx, metricsServer, time.Duration(10)*time.Second)

	go func() {
		klog.Infof("Listening on %s", addr)
		if err := server.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
			klog.Fatalf("Error listening: %q", err)
		}
	}()

	klog.Infof("Listening on %s for metrics", metricsAddr)
	if err := metricsServer.ListenAndServe(); err != http.ErrServerClosed {
		klog.Fatalf("Error listening: %q", err)
	}
	klog.Info("Graceflully closed")
}
