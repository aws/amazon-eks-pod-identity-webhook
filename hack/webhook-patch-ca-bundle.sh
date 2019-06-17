#!/bin/bash
# Original script found at: https://github.com/morvencao/kube-mutating-webhook-tutorial/blob/master/deployment/webhook-patch-ca-bundle.sh

ROOT=$(cd $(dirname $0)/../../; pwd)

set -o errexit
set -o nounset
set -o pipefail

secret_name=$(kubectl get sa default -o jsonpath='{.secrets[0].name}')


export CA_BUNDLE=$(kubectl get secret/$secret_name -o jsonpath='{.data.ca\.crt}' | tr -d '\n')

if command -v envsubst >/dev/null 2>&1; then
    envsubst
else
    sed -e "s|\${CA_BUNDLE}|${CA_BUNDLE}|g"
fi
