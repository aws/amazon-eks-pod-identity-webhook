module github.com/aws/amazon-eks-pod-identity-webhook

go 1.13

require (
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v0.9.3
	github.com/spf13/pflag v1.0.5
	gopkg.in/square/go-jose.v2 v2.5.1
	k8s.io/api v0.21.0
	k8s.io/apimachinery v0.21.0
	k8s.io/client-go v0.21.0
	k8s.io/klog v1.0.0
	sigs.k8s.io/yaml v1.2.0
)
