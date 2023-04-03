# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# See
# https://docs.docker.com/engine/reference/builder/#automatic-platform-args-in-the-global-scope
# for info on BUILDPLATFORM, TARGETOS, TARGETARCH, etc.
FROM --platform=$BUILDPLATFORM golang:1.19 AS builder
WORKDIR /go/src/github.com/aws/amazon-eks-pod-identity-webhook
COPY go.* .
ARG GOPROXY
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
RUN OS=$TARGETOS ARCH=$TARGETARCH make bin/github.com/aws/amazon-eks-pod-identity-webhook/webhook

# TODO: Is this the best base?
FROM public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base:latest.2 AS linux-amazon
COPY ATTRIBUTIONS.txt /ATTRIBUTIONS.txt
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/src/github.com/aws/amazon-eks-pod-identity-webhook/bin/github.com/aws/amazon-eks-pod-identity-webhook/webhook /bin/webhook
EXPOSE 443
VOLUME /etc/webhook
ENTRYPOINT ["/bin/webhook"]
CMD ["--logtostderr"]
