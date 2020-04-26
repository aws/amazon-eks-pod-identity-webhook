# AWS-specific make args
-include build/private/bgo_exports.makefile
include ${BGO_MAKEFILE}

export CGO_ENABLED=0
export T=github.com/aws/amazon-eks-pod-identity-webhook
UNAME_S = $(shell uname -s)
GO_LDFLAGS = -ldflags='-s -w -buildid=""'

install:: build
ifeq ($(UNAME_S), Darwin)
	GOOS=darwin GOARCH=amd64 go build -o build/gopath/bin/darwin_amd64/amazon-eks-pod-identity-webhook $(GO_LDFLAGS) $V $T
endif
	GOOS=linux GOARCH=amd64 go build -o build/gopath/bin/linux_amd64/amazon-eks-pod-identity-webhook $(GO_LDFLAGS) $V $T

# Generic make
REGISTRY_ID?=602401143452
IMAGE_NAME?=eks/pod-identity-webhook
REGION?=us-west-2
IMAGE?=$(REGISTRY_ID).dkr.ecr.$(REGION).amazonaws.com/$(IMAGE_NAME)

test:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

docker:
	@echo 'Building image $(IMAGE)...'
	docker build --no-cache -t $(IMAGE) .

push: docker
	if ! aws ecr get-login-password --region $(REGION) | docker login --username AWS --password-stdin $(REGISTRY_ID).dkr.ecr.$(REGION).amazonaws.com; then \
	  eval $$(aws ecr get-login --registry-ids $(REGISTRY_ID) --no-include-email); \
	fi
	docker push $(IMAGE)

amazon-eks-pod-identity-webhook:
	go build

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
	kubectl delete secret pod-identity-webhook

clean::
	rm -rf ./amazon-eks-pod-identity-webhook
	rm -rf ./certs/ coverage.out

.PHONY: docker push build local-serve local-request cluster-up cluster-down prep-config deploy-config delete-config clean


