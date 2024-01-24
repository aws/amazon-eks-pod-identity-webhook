# AWS-specific make args
-include build/private/bgo_exports.makefile
include ${BGO_MAKEFILE}

install:: build
	hack/install.sh

# Generic make
REGISTRY?=public.ecr.aws
IMAGE_NAME?=eks/pod-identity-webhook
IMAGE?=$(REGISTRY)/$(IMAGE_NAME)

GIT_COMMIT ?= $(shell git log -1 --pretty=%h)

# Architectures for binary builds
BIN_ARCH_LINUX ?= amd64 arm64

test:
	hack/test.sh

# Function build-image
# Parameters:
# 1: Target architecture
define build-image
$(MAKE) .image-linux-$(1)
endef

.PHONY: build-all-images
build-all-images:
	$(foreach arch,$(BIN_ARCH_LINUX),$(call build-image,$(arch)))

.PHONY: image
image: .image-linux-amd64

.PHONY: .image-linux-%
.image-linux-%:
	docker buildx build --output=type=docker --platform linux/$* \
		--build-arg golang_image=$(shell hack/setup-go.sh) --no-cache \
		--tag $(IMAGE):$(GIT_COMMIT)-linux_$* .

amazon-eks-pod-identity-webhook:
	hack/amazon-eks-pod-identity-webhook.sh

certs/tls.key:
	mkdir -p certs
	openssl req \
		-x509 \
		-newkey rsa:2048 \
		-keyout certs/tls.key \
		-out certs/tls.crt \
		-days 365 \
		-nodes \
		-subj "/CN=127.0.0.1"

local-serve: amazon-eks-pod-identity-webhook certs/tls.key
	./amazon-eks-pod-identity-webhook \
		--port 8443 \
		--in-cluster=false \
		--tls-key=./certs/tls.key \
		--tls-cert=./certs/tls.crt \
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
	@echo 'Deploying into active cluster...'
	cat deploy/deployment-base.yaml | sed -e "s|IMAGE|${IMAGE}|g" | tee deploy/deployment.yaml

deploy-config: prep-config
	@echo 'Applying configuration to active cluster...'
	kubectl apply -f deploy/auth.yaml
	kubectl apply -f deploy/deployment.yaml
	kubectl apply -f deploy/service.yaml
	kubectl apply -f deploy/mutatingwebhook.yaml

delete-config:
	@echo 'Tearing down mutating controller and associated resources...'
	kubectl delete -f deploy/service.yaml
	kubectl delete -f deploy/deployment.yaml
	kubectl delete -f deploy/auth.yaml
	kubectl delete -f deploy/mutatingwebhook.yaml
	kubectl delete secret pod-identity-webhook-cert

clean::
	rm -rf ./amazon-eks-pod-identity-webhook
	rm -rf ./certs/ coverage.out

.PHONY: image build local-serve local-request cluster-up cluster-down prep-config deploy-config delete-config clean


