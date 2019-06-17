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
        "StringEquals": {
         # scope the role to your cluster [required]
         "oidc.us-west-2.eks.amazonaws.com/624a142e-43fc-4a4e-9a65-0adbfe9d6a85:aud": "sts.amazonaws.com",
         # scope the role to the service account (optional)
         "oidc.us-west-2.eks.amazonaws.com/624a142e-43fc-4a4e-9a65-0adbfe9d6a85:sub": "system:serviceaccount:default:my-serviceaccount"
        },
        # Optional for scoping to a namespace
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
        - name: AWS_IAM_ROLE_ARN
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
      --cert-dir string                  (out-of-cluster) Directory to save certificates (default "/etc/webhook/certs")
      --cert-duration duration           (out-of-cluster) Lifetime for self-signed certificate (default 8760h0m0s)
      --in-cluster                       Use in-cluster auth (default true)
      --kube-api string                  (out-of-cluster) The url to the API server
      --kubeconfig string                (out-of-cluster) Absolute path to the API server kubeconfig file (default "~/.kube/config")
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --logtostderr                      log to standard error instead of files
      --namespace string                 (in-cluster) The namespace name this webhook and the tls secret resides in (default "eks")
      --port int                         Port to listen on (default 443)
      --service-name string              (in-cluster) The service name fronting this webhook (default "iam-for-pods")
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
      --tls-secret string                (in-cluster) The secret name for storing the TLS serving cert (default "iam-for-pods")
      --token-audience string            The default audience for tokens. Can be overridden by annotation (default "sts.amazonaws.com")
      --token-expiration int             The token expiration (default 86400)
      --token-mount-path string          The path to mount tokens (default "/var/run/secrets/eks.amazonaws.com/serviceaccount")
  -v, --v Level                          log level for V logs
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
      --webhook-config string            (out-of-cluster) Path for where to write the webhook config file for the API server to consume (default "/etc/webhook/config.yaml")
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
TODO

## License
TODO
