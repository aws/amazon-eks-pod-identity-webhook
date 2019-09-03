# Amazon EKS Pod Identity Webhook

This webhook is for mutating pods that will require AWS IAM access.

## EKS Walkthrough

1. [Create an OIDC provider][1] in IAM for your cluster. You can find the OIDC
   discovery endpoint by describing your EKS cluster.
    ```bash
    aws eks describe-cluster --name $CLUSTER_NAME --query cluster.tokenDiscoveryEndpoint
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
        "Federated": "arn:aws:iam::111122223333:oidc-provider/oidc.us-west-2.eks.amazonaws.com/624a142e-43fc-4a4e-9a65-0adbfe9d6a85"
       },
       "Action": "sts:AssumeRoleWithWebIdentity",
       "Condition": {
        "__doc_comment": "scope the role to the service account (optional)",
        "StringEquals": {
         "oidc.us-west-2.eks.amazonaws.com/624a142e-43fc-4a4e-9a65-0adbfe9d6a85:sub": "system:serviceaccount:default:my-serviceaccount"
        },
        "__doc_comment": "scope the role to a namespace (optional)",
        "StringLike": {
         "oidc.us-west-2.eks.amazonaws.com/624a142e-43fc-4a4e-9a65-0adbfe9d6a85:sub": "system:serviceaccount:default:*"
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
    ```
4. All new pod pods launched using this Service Account will be modified to use
   IAM for pods. Below is an example pod spec with the environment variables and
   volume fields added by the webhook.
    ```yaml
    apiVersion: v1
    kind: Pod
    metadata:
      name: my-pod
      namespace: defaut
    spec:
      serviceAccountName: my-serviceaccount
      containers:
      - name: container-name
        image: container-image:version
    ### Everything below is added by the webhook ###
        env:
        - name: AWS_ROLE_ARN
          value: "arn:aws:iam::111122223333:role/s3-reader"
        - name: AWS_WEB_IDENTITY_TOKEN_FILE
          value: "/var/run/secrets/eks.amazonaws.com/serviceaccount/token"
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

[1]: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_create_oidc.html
[2]: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-idp_oidc.html

## Usage

```
Usage of amazon-eks-pod-identity-webhook:
      --alsologtostderr                  log to standard error as well as files
      --annotation-prefix string         The Service Account annotation to look for (default "eks.amazonaws.com")
      --in-cluster                       Use in-cluster authentication and certificate request API (default true)
      --kube-api string                  (out-of-cluster) The url to the API server
      --kubeconfig string                (out-of-cluster) Absolute path to the API server kubeconfig file
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --log_file string                  If non-empty, use this log file
      --log_file_max_size uint           Defines the maximum size a log file can grow to. Unit is megabytes. If the value is 0, the maximum file size is unlimited. (default 1800)
      --logtostderr                      log to standard error instead of files (default true)
      --namespace string                 (in-cluster) The namespace name this webhook and the tls secret resides in (default "eks")
      --port int                         Port to listen on (default 443)
      --service-name string              (in-cluster) The service name fronting this webhook (default "pod-identity-webhook")
      --skip_headers                     If true, avoid header prefixes in the log messages
      --skip_log_headers                 If true, avoid headers when openning log files
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
      --tls-cert string                  (out-of-cluster) TLS certificate file path (default "/etc/webhook/certs/tls.cert")
      --tls-key string                   (out-of-cluster) TLS key file path (default "/etc/webhook/certs/tls.key")
      --tls-secret string                (in-cluster) The secret name for storing the TLS serving cert (default "pod-identity-webhook")
      --token-audience string            The default audience for tokens. Can be overridden by annotation (default "sts.amazonaws.com")
      --token-expiration int             The token expiration (default 86400)
      --token-mount-path string          The path to mount tokens (default "/var/run/secrets/eks.amazonaws.com/serviceaccount")
  -v, --v Level                          number for the log level verbosity
      --version                          Display the version and exit
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

## Installation

### In-cluster

You can use the provided configuration files in the `deploy` directory, along with the provided `Makefile`

```
make cluster-up IMAGE=602401143452.dkr.ecr.us-west-2.amazonaws.com/eks/pod-identity-webhook:latest
```

This will:
* Create a service account, role, cluster-role, role-binding, and cluster-role-binding that will the deployment requires
* Create the deployment, service, and mutating webhook in the cluster
* Approve the CSR that the deployment created for its TLS serving certificate

### On API server
TODO

## Development
TODO

## Code of Conduct
See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)

## License
Apache 2.0 - Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
See [LICENSE](LICENSE)
