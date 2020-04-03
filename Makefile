# AWS-specific make args
-include build/private/bgo_exports.makefile
include ${BGO_MAKEFILE}

export CGO_ENABLED=0
export T=github.com/aws/amazon-eks-pod-identity-webhook
UNAME_S = $(shell uname -s)
GO_INSTALL_FLAGS = -ldflags="-s -w"

install:: build
ifeq ($(UNAME_S), Darwin)
	GOOS=darwin GOARCH=amd64 go build -o build/gopath/bin/darwin_amd64/amazon-eks-pod-identity-webhook $(GO_INSTALL_FLAGS) $V $T
endif
	GOOS=linux GOARCH=amd64 go build -o build/gopath/bin/linux_amd64/amazon-eks-pod-identity-webhook $(GO_INSTALL_FLAGS) $V $T


# Generic make
REGISTRY_ID?=602401143452
IMAGE_NAME?=eks/pod-identity-webhook
REGION?=us-west-2
IMAGE?=$(REGISTRY_ID).dkr.ecr.$(REGION).amazonaws.com/$(IMAGE_NAME)

docker:
	@echo 'Building image $(IMAGE)...'
	docker build --no-cache -t $(IMAGE) .

push: docker
	eval $$(aws ecr get-login --registry-ids $(REGISTRY_ID) --no-include-email)
	docker push $(IMAGE)

amazon-eks-pod-identity-webhook:
	go build

certs/tls.key:
	mkdir -p certs
	openssl req \
		-x509 \
		-newkey rsa:2048 \
		-keyout certs/tls.key \
		-out certs/tls.cert \
		-days 365 \
		-nodes \
		-subj "/CN=127.0.0.1"

local-serve: amazon-eks-pod-identity-webhook certs/tls.key
	./amazon-eks-pod-identity-webhook \
		--port 8443 \
		--in-cluster=false \
		--tls-key=./certs/tls.key \
		--tls-cert=./certs/tls.cert \
		--kubeconfig=$$HOME/.kube/config

local-request:
	@curl \
		-s \
		-k \
		-H "Content-Type: application/json" \
		-X POST \
		-d @hack/request.json \
		https://localhost:8443/mutate | jq

# cluster commands
cluster-up: deploy-config

cluster-down: delete-config

prep-config:
	@echo 'Generating certs and deploying into active cluster...'
	cat deploy/deployment-base.yaml | sed -e "s|IMAGE|${IMAGE}|g" | tee deploy/deployment.yaml
	cat deploy/mutatingwebhook.yaml | hack/webhook-patch-ca-bundle.sh > deploy/mutatingwebhook-ca-bundle.yaml

deploy-config: prep-config
	@echo 'Applying configuration to active cluster...'
	kubectl apply -f deploy/auth.yaml
	kubectl apply -f deploy/deployment.yaml
	kubectl apply -f deploy/service.yaml
	kubectl apply -f deploy/mutatingwebhook-ca-bundle.yaml
	until kubectl get csr -o \
		jsonpath='{.items[?(@.spec.username=="system:serviceaccount:default:pod-identity-webhook")].metadata.name}' | \
		grep -m 1 "csr-"; \
		do echo "Waiting for CSR to be created" && sleep 1 ; \
	done
	kubectl certificate approve $$(kubectl get csr -o jsonpath='{.items[?(@.spec.username=="system:serviceaccount:default:pod-identity-webhook")].metadata.name}')

delete-config:
	@echo 'Tearing down mutating controller and associated resources...'
	kubectl delete -f deploy/mutatingwebhook-ca-bundle.yaml
	kubectl delete -f deploy/service.yaml
	kubectl delete -f deploy/deployment.yaml
	kubectl delete -f deploy/auth.yaml

clean::
	rm -rf ./amazon-eks-pod-identity-webhook
	rm -rf ./certs/

.PHONY: docker push build local-serve local-request cluster-up cluster-down prep-config deploy-config delete-config clean


