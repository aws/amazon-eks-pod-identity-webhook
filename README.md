![build](https://github.com/aws/amazon-eks-pod-identity-webhook/workflows/build/badge.svg)

# Amazon EKS Pod Identity Webhook

This webhook is for mutating pods that will require AWS IAM access.

## Note
After version v0.3.0, `--in-cluster=true` no longer works and is deprecated.  Please use `--in-cluster=false`
and manage the cluster certificate with cert-manager or some other external certificate provisioning system.
This is because certificates using the `legacy-unknown` signer are no longer signed when using the v1
certificates API.

## EKS Walkthrough

1. [Create an OIDC provider][1] in IAM for your cluster. You can find the OIDC
   discovery endpoint by describing your EKS cluster.
    ```bash
    aws eks describe-cluster --name $CLUSTER_NAME --query cluster.identity.oidc
    ```
    And enter "sts.amazonaws.com" as the client-id
2. Create an IAM role for your pods and [modify the trust policy][2] to allow
   your pod's service account to use the role:
    ```json
    {
     "Version": "2012-10-17",
     "Statement": [
      {
       "Effect": "Allow",
       "Principal": {
        "Federated": "arn:aws:iam::111122223333:oidc-provider/oidc.REGION.eks.amazonaws.com/CLUSTER_ID"
       },
       "Action": "sts:AssumeRoleWithWebIdentity",
       "Condition": {
        "__doc_comment": "scope the role to the service account (optional)",
        "StringEquals": {
         "oidc.REGION.eks.amazonaws.com/CLUSTER_ID:sub": "system:serviceaccount:default:my-serviceaccount"
        },
        "__doc_comment": "scope the role to a namespace (optional)",
        "StringLike": {
         "oidc.REGION.eks.amazonaws.com/CLUSTER_ID:sub": "system:serviceaccount:default:*"
        }
       }
      }
     ]
    }
    ```
3. Modify your pod's service account to be annotated with the ARN of the role
   you want the pod to use
    ```yaml
    apiVersion: v1
    kind: ServiceAccount
    metadata:
      name: my-serviceaccount
      namespace: default
      annotations:
        eks.amazonaws.com/role-arn: "arn:aws:iam::111122223333:role/s3-reader"
        # optional: Defaults to "sts.amazonaws.com" if not set
        eks.amazonaws.com/audience: "sts.amazonaws.com"
        # optional: When set to "true", adds AWS_STS_REGIONAL_ENDPOINTS env var
        #   to containers
        eks.amazonaws.com/sts-regional-endpoints: "true"
        # optional: Defaults to 86400 for expirationSeconds if not set
        #   Note: This value can be overwritten if specified in the pod 
        #         annotation as shown in the next step.
        eks.amazonaws.com/token-expiration: "86400"
    ```
4. All new pod pods launched using this Service Account will be modified to use
   IAM for pods. Below is an example pod spec with the environment variables and
   volume fields added by the webhook.
    ```yaml
    apiVersion: v1
    kind: Pod
    metadata:
      name: my-pod
      namespace: default
      annotations:
        # optional: A comma-separated list of initContainers and container names
        #   to skip adding volumes and environment variables
        eks.amazonaws.com/skip-containers: "init-first,sidecar"
        # optional: Defaults to 86400, or value specified in ServiceAccount
        #   annotation as shown in previous step, for expirationSeconds if not set
        eks.amazonaws.com/token-expiration: "86400"
    spec:
      serviceAccountName: my-serviceaccount
      initContainers:
      - name: init-first
        image: container-image:version
      containers:
      - name: sidecar
        image: container-image:version
      - name: container-name
        image: container-image:version
    ### Everything below is added by the webhook ###
        env:
        - name: AWS_DEFAULT_REGION
          value: us-west-2
        - name: AWS_REGION
          value: us-west-2
        - name: AWS_ROLE_ARN
          value: "arn:aws:iam::111122223333:role/s3-reader"
        - name: AWS_WEB_IDENTITY_TOKEN_FILE
          value: "/var/run/secrets/eks.amazonaws.com/serviceaccount/token"
        - name: AWS_STS_REGIONAL_ENDPOINTS
          value: "regional"
        volumeMounts:
        - mountPath: "/var/run/secrets/eks.amazonaws.com/serviceaccount/"
          name: aws-token
      volumes:
      - name: aws-token
        projected:
          sources:
          - serviceAccountToken:
              audience: "sts.amazonaws.com"
              expirationSeconds: 86400
              path: token
    ```

### Usage with Windows container workloads

To ensure workloads are scheduled on windows nodes have the right environment variables, they must have a `nodeSelector` targeting windows it must run on.  Workloads targeting windows nodes using `nodeAffinity` are currently not supported.
```yaml
  nodeSelector:
    beta.kubernetes.io/os: windows
```

Or for Kubernetes 1.14+

```yaml
  nodeSelector:
    kubernetes.io/os: windows
```



[1]: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_create_oidc.html
[2]: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-idp_oidc.html

### Usage with non-root container user

When running a container with a non-root user, you need to give the container access to the token file by setting the `fsGroup` field in the `securityContext` object.

## Usage

```
Usage of amazon-eks-pod-identity-webhook:
      --alsologtostderr                  log to standard error as well as files
      --annotation-prefix string         The Service Account annotation to look for (default "eks.amazonaws.com")
      --aws-default-region string        If set, AWS_DEFAULT_REGION and AWS_REGION will be set to this value in mutated containers
      --in-cluster                       Use in-cluster authentication and certificate request API (default true)
      --enable-debugging-handlers        Enable debugging handlers on the metrics port (http). Currently /debug/alpha/cache is supported (default false) [ALPHA]
      --kube-api string                  (out-of-cluster) The url to the API server
      --kubeconfig string                (out-of-cluster) Absolute path to the API server kubeconfig file
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --log_file string                  If non-empty, use this log file
      --log_file_max_size uint           Defines the maximum size a log file can grow to. Unit is megabytes. If the value is 0, the maximum file size is unlimited. (default 1800)
      --logtostderr                      log to standard error instead of files (default true)
      --metrics-port int                 Port to listen on for metrics and healthz (http) (default 9999)
      --namespace string                 (in-cluster) The namespace name this webhook and the tls secret resides in (default "eks")
      --port int                         Port to listen on (default 443)
      --service-name string              (in-cluster) The service name fronting this webhook (default "pod-identity-webhook")
      --skip_headers                     If true, avoid header prefixes in the log messages
      --skip_log_headers                 If true, avoid headers when openning log files
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
      --sts-regional-endpoint false      Whether to inject the AWS_STS_REGIONAL_ENDPOINTS=regional env var in mutated pods. Defaults to false.
      --tls-cert string                  (out-of-cluster) TLS certificate file path (default "/etc/webhook/certs/tls.crt")
      --tls-key string                   (out-of-cluster) TLS key file path (default "/etc/webhook/certs/tls.key")
      --tls-secret string                (in-cluster) The secret name for storing the TLS serving cert (default "pod-identity-webhook")
      --token-audience string            The default audience for tokens. Can be overridden by annotation (default "sts.amazonaws.com")
      --token-expiration int             The token expiration (default 86400)
      --token-mount-path string          The path to mount tokens (default "/var/run/secrets/eks.amazonaws.com/serviceaccount")
  -v, --v Level                          number for the log level verbosity
      --version                          Display the version and exit
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

### AWS_DEFAULT_REGION Injection

When the `aws-default-region` flag is set this webhook will inject `AWS_DEFAULT_REGION` and `AWS_REGION` in mutated containers if `AWS_DEFAULT_REGION` and `AWS_REGION` are not already set.

### AWS_STS_REGIONAL_ENDPOINTS Injection

When the `sts-regional-endpoint` flag is set to `true`, the webhook will
inject the environment variable `AWS_STS_REGIONAL_ENDPOINTS` with the value set
to `regional`. This environment variable will configure the AWS SDKs to perform
the `sts:AssumeRoleWithWebIdentity` call to get credentials from the regional
endpoint, instead of the global endpoint in `us-east-1`. This is desirable in
almost all cases, unless the STS regional endpoint is [disabled in your
account](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp_enable-regions.html).

You can also enable this per-service account with the annotation
`eks.amazonaws.com/sts-regional-endpoints` set to `"true"`.

## Container Images

Container images for amazon-eks-pod-identity-webhook can be found on [Docker Hub](https://hub.docker.com/r/amazon/amazon-eks-pod-identity-webhook).

## Installation

### Pre-requisites

You must install cert-manager as it is a pre-requisite for below deployments. (See [cert-manager installation](https://cert-manager.io/docs/installation/))

### In-cluster

You can use the provided configuration files in the `deploy` directory, along with the provided `Makefile`.

```
make cluster-up IMAGE=amazon/amazon-eks-pod-identity-webhook:latest
```

This will:
* Create a service account, role, cluster-role, role-binding, and cluster-role-binding that the deployment requires
* Create the deployment, service, ClusterIssuer, certificate, and mutating webhook in the cluster
* Use `in-cluster=false` so that the webhook reloads certificates from the filesystem rather than creating CSRs to request certificates (using CSRs is now deprecated and will not work versions later than v0.3.0).

For self-hosted API server configuration, see see [SELF_HOSTED_SETUP.md](/SELF_HOSTED_SETUP.md)

### On API server
TODO

### Notes
With the upgrade to client-go 1.18, certificate_manager_server_expiration_seconds metric has been removed by an upstream commit kubernetes/kubernetes#85874.
A new metric certificate_manager_server_rotation_seconds is added which tracks the time a certificate was valid before getting rotated.

## Code of Conduct
See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)

## License
Apache 2.0 - Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
See [LICENSE](LICENSE)

