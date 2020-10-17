![build](https://github.com/aws/amazon-eks-pod-identity-webhook/workflows/build/badge.svg)

# Amazon EKS Pod Identity Webhook

This webhook is for mutating pods that will require AWS IAM access.

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
      namespace: default
    spec:
      serviceAccountName: my-serviceaccount
      containers:
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

## Usage

```
Usage of amazon-eks-pod-identity-webhook:
      --alsologtostderr                  log to standard error as well as files
      --annotation-prefix string         The Service Account annotation to look for (default "eks.amazonaws.com")
      --aws-default-region string        If set, AWS_DEFAULT_REGION and AWS_REGION will be set to this value in mutated containers
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


## Container Images

Container images for amazon-eks-pod-identity-webhook can be found on [Docker Hub](https://hub.docker.com/r/amazon/amazon-eks-pod-identity-webhook).

## Installation

### In-cluster

You can use the provided configuration files in the `deploy` directory, along with the provided `Makefile`

```
make cluster-up IMAGE=amazon/amazon-eks-pod-identity-webhook:2db5e53
```

This will:
* Create a service account, role, cluster-role, role-binding, and cluster-role-binding that will the deployment requires
* Create the deployment, service, and mutating webhook in the cluster
* Approve the CSR that the deployment created for its TLS serving certificate

For self-hosted API server configuration, see see [SELF_HOSTED_SETUP.md](/SELF_HOSTED_SETUP.md)

### On API server
TODO

## Development
TODO

## Guides

### Different roles in the same pod

You can use different IAM roles in different containers within the same pod, but
there is additional setup and configuration that you'll have to modify your pod
spec by adding the volumes and environment variables without the help of this
webhook.

The way this works is by using different client IDs (also known as audience) in
different tokens. Below is an example pod, but you'll probably use this on a
Deployment, Daemonset, or some other controller type.

First, you'll need to modify your IAM OIDC Identity Provider (IDP) to add an
additional audience/client ID. In this example, the IDP will use the audiences
`sts.amazonaws.com` and `some-other-client-id-arbitrary-string`.

**Note**: _Containers in a pod share a common kernel, and containers are not a
hard security boundary. Using different AWS IAM roles for containers in the same
pod can be used to reduce unnecesary permissions and to reduce the blast-radius
of misconfiguration, but should not be relied on to segment untrusted code in
running in a container._

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  namespace: default
spec:
  serviceAccountName: my-serviceaccount
  containers:
  - name: container1
    image: container-image1
    env:
    - name: AWS_ROLE_ARN
      value: "arn:aws:iam::111122223333:role/role-1"
    - name: AWS_WEB_IDENTITY_TOKEN_FILE
      value: "/var/run/secrets/eks.amazonaws.com/serviceaccount/token"
    volumeMounts:
    - mountPath: "/var/run/secrets/eks.amazonaws.com/serviceaccount/"
      name: aws-token-1
  - name: container2
    image: container-image2
    env:
    - name: AWS_ROLE_ARN
      value: "arn:aws:iam::111122223333:role/role-2"
    - name: AWS_WEB_IDENTITY_TOKEN_FILE
      value: "/var/run/secrets/eks.amazonaws.com/serviceaccount/token"
    volumeMounts:
    - mountPath: "/var/run/secrets/eks.amazonaws.com/serviceaccount/"
      name: aws-token-2
  volumes:
  - name: aws-token-1
    projected:
      sources:
      - serviceAccountToken:
          audience: "sts.amazonaws.com"
          expirationSeconds: 86400
          path: token
  - name: aws-token-2
    projected:
      sources:
      - serviceAccountToken:
          audience: "some-other-client-id-arbitrary-string"
          expirationSeconds: 86400
          path: token
```

Now for each role, you'll need to add a key to the `StringEquals` condition map
in the role's trust policy specifying the proper audience (`aud`) for each role.

`role-1` trust policy:

```json
{
 "Version": "2012-10-17",
 "Statement": [
  {
   "Effect": "Allow",
   "Principal": {
    "Federated": "arn:aws:iam::ACCOUNT_ID:oidc-provider/oidc.REGION.eks.amazonaws.com/CLUSTER_ID"
   },
   "Action": "sts:AssumeRoleWithWebIdentity",
   "Condition": {
    "StringEquals": {
     "oidc.REGION.eks.amazonaws.com/CLUSTER_ID:sub": "system:serviceaccount:default:my-serviceaccount",
     "oidc.REGION.eks.amazonaws.com/CLUSTER_ID:aud": "sts.amazonaws.com"
    }
   }
  }
 ]
}
```

And `role-2` trust policy:

```json
{
 "Version": "2012-10-17",
 "Statement": [
  {
   "Effect": "Allow",
   "Principal": {
    "Federated": "arn:aws:iam::ACCOUNT_ID:oidc-provider/oidc.REGION.eks.amazonaws.com/CLUSTER_ID"
   },
   "Action": "sts:AssumeRoleWithWebIdentity",
   "Condition": {
    "StringEquals": {
     "oidc.REGION.eks.amazonaws.com/CLUSTER_ID:sub": "system:serviceaccount:default:my-serviceaccount",
     "oidc.REGION.eks.amazonaws.com/CLUSTER_ID:aud": "some-other-client-id-arbitrary-string"
    }
   }
  }
 ]
}
```

## Code of Conduct
See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)

## License
Apache 2.0 - Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
See [LICENSE](LICENSE)
