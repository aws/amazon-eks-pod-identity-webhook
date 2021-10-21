# Self-hosted Kubernetes setup

If you are running your own Kubernetes cluster, there are several steps required for this feature to work.

## Prerequisites

1. Your cluster must be running Kubernetes 1.12 or later.

2. Your cluster's `kube-controller-manager` must be properly configured to sign
   certificate requests. You can verify this by validating that the
   `--cluster-signing-cert-file` and `--cluster-signing-key-file` parameters
   point to valid TLS certificate and key files. See [this
   document](https://kubernetes.io/docs/tasks/tls/managing-tls-in-a-cluster) for
   details.

## Projected Token Signing Keypair

The first thing required is a new key pair for signing and verifying projected
service account tokens. This can be done using the following `ssh-keygen`
commands.

```bash
# Generate the keypair
PRIV_KEY="sa-signer.key"
PUB_KEY="sa-signer.key.pub"
PKCS_KEY="sa-signer-pkcs8.pub"
# Generate a key pair
ssh-keygen -t rsa -b 2048 -f $PRIV_KEY -m pem
# convert the SSH pubkey to PKCS8
ssh-keygen -e -m PKCS8 -f $PUB_KEY > $PKCS_KEY
```

## Public Issuer

As of 1.16, Kubernetes does not include an OIDC discovery endpoint itself (see
[kubernetes/community#1190](https://github.com/kubernetes/enhancements/pull/1190)),
so you will need to put your public signing key somewhere that AWS STS can
discover it. This example, we will create one in a public S3 bucket, but you
could host the following documents any way you'd like on a different domain.

### Create an S3 bucket

```bash
# Create S3 bucket with a random name. Feel free to set your own name here
export S3_BUCKET=${S3_BUCKET:-oidc-test-$(cat /dev/random | LC_ALL=C tr -dc "[:alpha:]" | tr '[:upper:]' '[:lower:]' | head -c 32)}
# Create the bucket if it doesn't exist
_bucket_name=$(aws s3api list-buckets  --query "Buckets[?Name=='$S3_BUCKET'].Name | [0]" --out text)
if [ $_bucket_name == "None" ]; then
    if [ "$AWS_REGION" == "us-east-1" ]; then
        aws s3api create-bucket --bucket $S3_BUCKET
    else
        aws s3api create-bucket --bucket $S3_BUCKET --create-bucket-configuration LocationConstraint=$AWS_REGION
    fi
fi
echo "export S3_BUCKET=$S3_BUCKET"
export HOSTNAME=s3-$AWS_REGION.amazonaws.com
export ISSUER_HOSTPATH=$HOSTNAME/$S3_BUCKET
```

### Create the OIDC discovery and keys documents

Part of the OIDC spec is to host an OIDC discovery and a keys JSON document.
Lets create these:

```bash
cat <<EOF > discovery.json
{
    "issuer": "https://$ISSUER_HOSTPATH",
    "jwks_uri": "https://$ISSUER_HOSTPATH/keys.json",
    "authorization_endpoint": "urn:kubernetes:programmatic_authorization",
    "response_types_supported": [
        "id_token"
    ],
    "subject_types_supported": [
        "public"
    ],
    "id_token_signing_alg_values_supported": [
        "RS256"
    ],
    "claims_supported": [
        "sub",
        "iss"
    ]
}
EOF
```

Included in this repo is a small go file to help create the keys json document.

```bash
go run ./hack/self-hosted/main.go -key $PKCS_KEY  | jq '.keys += [.keys[0]] | .keys[1].kid = ""' > keys.json
```

**note:** This will print the same key twice, once with an empty `kid` and once
populated. Prior to Kubernetes 1.16 (PR [#78502](https://github.com/kubernetes/kubernetes/pull/78502))
the API server did not add a `kid` value to projected tokens. In 1.16+, the
`kid` is included. By printing the key twice, you can safely upgrade a cluster
to 1.16. Graceful signing key rotation is not possible prior to 1.16 since
tokens were always signed with the same empty `kid` value, even if they used
different public keys.

After you have the `keys.json` and `discovery.json` files, you'll need to place
them in your bucket. It is critical these objects are public so STS can access
them.

```bash
aws s3 cp --acl public-read ./discovery.json s3://$S3_BUCKET/.well-known/openid-configuration
aws s3 cp --acl public-read ./keys.json s3://$S3_BUCKET/keys.json
```

## Kubernetes API Server configuration

As of Kubernetes 1.12, Kubernetes can issue and mount projected service account
tokens in pods.

In order to use this feature, you'll need to set the following
[API server flags](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-apiserver/).

```
# Path to the $PKCS_KEY file from the beginning. 
#
# This flag can be specified for multiple times.
# There is likely already one specified for legacy service accounts, if not, 
# it is using the default value. Find out your default value and pass it explicitly
# (along with this $PKCS_KEY), otherwise your existing tokens will fail.
--service-account-key-file

# Path to the signing (private) key ($PRIV_KEY)
--service-account-signing-key-file

# Identifiers of the API. The service account token authenticator will validate
# that tokens used against the API are bound to at least one of these audiences.
# If the --service-account-issuer flag is configured and this flag is not, this
# field defaults to a single element list containing the issuer URL.
#
# `--api-audiences` is for v1.13+, `--service-account-api-audiences` in v1.12
--api-audiences

# The issuer URL, or "https://$ISSUER_HOSTPATH" from above.
--service-account-issuer
```

## Audiences

The above `--api-audiences` flag sets an `aud` value for tokens that do not
request an audience, and the API server requires that any projected tokens used
for pod to API server authentication must have this audience set. This can
usually be set to `kubernetes.svc.default`, or optionally the DNS name of your
API server.

When using a Kubernetes-issued token for an external system, you should use a
different audience (or in OAuth2 parlance, `client-id`). The external system
(such as AWS IAM) will usually require an audience, or client-id, at setup. For
AWS IAM, a token's `aud` must match the OIDC Identity Provider's client ID. EKS
uses the string `sts.amazonaws.com` as the default, but when using the webhook
yourself, you can use any audience you'd like as long as the webhook's flag
`--token-audience` is set to the same value as your IDP in IAM.

## Provider creation

From here, you can mostly follow the process in the [EKS
documentation](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)
and substitute the cluster issuer with `https://$ISSUER_HOSTPATH`.

## Deploying the webhook

Follow the steps in the [In-cluster installation](https://github.com/aws/amazon-eks-pod-identity-webhook#in-cluster) section to launch the webhook
and its required resources in the cluster.

## Troubleshooting

### `Certificate request was not signed: timed out waiting for the condition` appears in the logs

Check the output of `kubectl get -n <namespace> csr | grep pod-identity-webhook`.
The last column should contain `Approved,Issued` as in the following example:

```
csr-869cl   2m52s   system:serviceaccount:<namespace>:pod-identity-webhook   Approved,Issued
```

If it says `Approved` but not `Issued`, your cluster's controller manager is
likely not configured properly as a TLS Certificate Authority.  Please review
the Prerequisites section above.  You will need to restart the controller
manager.
