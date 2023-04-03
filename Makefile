# AWS-specific make args
-include build/private/bgo_exports.makefile
include ${BGO_MAKEFILE}

test:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

VERSION?=v0.4.0

PKG=github.com/aws/amazon-eks-pod-identity-webhook
GIT_COMMIT?=$(shell git rev-parse HEAD)
BUILD_DATE?=$(shell date -u -Iseconds)
LDFLAGS?="-X ${PKG}/pkg/driver.driverVersion=${VERSION} -X ${PKG}/pkg/cloud.driverVersion=${VERSION} -X ${PKG}/pkg/driver.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/driver.buildDate=${BUILD_DATE} -s -w"


GO111MODULE=on
GOPATH=$(shell go env GOPATH)
GOOS=$(shell go env GOOS)
GOBIN=$(shell pwd)/bin

# TODO: Is this still an appropriate default?
REGISTRY?=gcr.io/k8s-staging-provider-aws
IMAGE?=$(REGISTRY)/amazon-eks-pod-identity-webhook
TAG?=$(GIT_COMMIT)

OUTPUT_TYPE?=docker

OS?=linux
ARCH?=amd64
OSVERSION?=amazon

ALL_OS?=linux
ALL_ARCH_linux?=amd64 arm64
ALL_OSVERSION_linux?=amazon
ALL_OS_ARCH_OSVERSION_linux=$(foreach arch, $(ALL_ARCH_linux), $(foreach osversion, ${ALL_OSVERSION_linux}, linux-$(arch)-${osversion}))

ALL_OS_ARCH_OSVERSION=$(foreach os, $(ALL_OS), ${ALL_OS_ARCH_OSVERSION_${os}})

# split words on hyphen, access by 1-index
word-hyphen = $(word $2,$(subst -, ,$1))

.EXPORT_ALL_VARIABLES:

.PHONY: linux/$(ARCH) bin/${PKG}/amazon-eks-pod-identity-webhook
linux/$(ARCH): bin/${PKG}/amazon-eks-pod-identity-webhook
bin/${PKG}/amazon-eks-pod-identity-webhook: | bin
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -mod=mod -ldflags ${LDFLAGS} -o bin/${PKG}/webhook

# Builds all linux and windows images and pushes them
.PHONY: all-push
all-push: all-image-registry push-manifest

.PHONY: push-manifest
push-manifest: create-manifest
	docker manifest push --purge $(IMAGE):$(TAG)

.PHONY: create-manifest
create-manifest: all-image-registry
# sed expression:
# LHS: match 0 or more not space characters
# RHS: replace with $(IMAGE):$(TAG)-& where & is what was matched on LHS
	docker manifest create --amend $(IMAGE):$(TAG) $(shell echo $(ALL_OS_ARCH_OSVERSION) | sed -e "s~[^ ]*~$(IMAGE):$(TAG)\-&~g")

# Only linux for OUTPUT_TYPE=docker because windows image cannot be exported
# "Currently, multi-platform images cannot be exported with the docker export type. The most common usecase for multi-platform images is to directly push to a registry (see registry)."
# https://docs.docker.com/engine/reference/commandline/buildx_build/#output
.PHONY: all-image-docker
all-image-docker: $(addprefix sub-image-docker-,$(ALL_OS_ARCH_OSVERSION_linux))
.PHONY: all-image-registry
all-image-registry: $(addprefix sub-image-registry-,$(ALL_OS_ARCH_OSVERSION))

sub-image-%:
	$(MAKE) OUTPUT_TYPE=$(call word-hyphen,$*,1) OS=$(call word-hyphen,$*,2) ARCH=$(call word-hyphen,$*,3) OSVERSION=$(call word-hyphen,$*,4) image

.PHONY: image
image: .image-$(TAG)-$(OS)-$(ARCH)-$(OSVERSION)
.image-$(TAG)-$(OS)-$(ARCH)-$(OSVERSION):
	docker buildx build \
		--platform=$(OS)/$(ARCH) \
		--progress=plain \
		--target=$(OS)-$(OSVERSION) \
		--output=type=$(OUTPUT_TYPE) \
		-t=$(IMAGE):$(TAG)-$(OS)-$(ARCH)-$(OSVERSION) \
		--build-arg=GOPROXY=$(GOPROXY) \
		--build-arg=VERSION=$(VERSION) \
		`./hack/provenance` \
		.
	touch $@

amazon-eks-pod-identity-webhook: bin/${PKG}/amazon-eks-pod-identity-webhook

build: amazon-eks-pod-identity-webhook
	cp bin/${PKG}/webhook ./amazon-eks-pod-identity-webhook

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
	rm -rf .*image-*
	rm -rf ./bin/
	rm -rf ./amazon-eks-pod-identity-webhook
	rm -rf ./certs/ coverage.out

bin:
	@mkdir -p $@

.PHONY: docker push build local-serve local-request cluster-up cluster-down prep-config deploy-config delete-config clean


