# AWS-specific make args
-include build/private/bgo_exports.makefile
include ${BGO_MAKEFILE}

GO_INSTALL_FLAGS=-ldflags="-s -w"

# Generic make
REGISTRY_ID?=602401143452
IMAGE_NAME?=eks/iam-for-pods
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

serve-local: amazon-eks-pod-identity-webhook
	./amazon-eks-pod-identity-webhook \
		--port 8443 \
		--in-cluster=false

local-request:
	curl \
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
	sleep 1
	kubectl certificate approve $$(kubectl get csr -o jsonpath='{.items[?(@.spec.username=="system:serviceaccount:eks:iam-for-pods")].metadata.name}') \

delete-config:
	@echo 'Tearing down mutating controller and associated resources...'
	kubectl delete -f deploy/mutatingwebhook-ca-bundle.yaml
	kubectl delete -f deploy/service.yaml
	kubectl delete -f deploy/deployment.yaml
	kubectl delete -f deploy/auth.yaml
