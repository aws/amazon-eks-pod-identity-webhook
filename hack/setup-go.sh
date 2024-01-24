#!/bin/bash
# Copyright 2020 The Kubernetes Authors.
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

# script to setup go version as needed
# MUST BE RUN FROM THE REPO ROOT DIRECTORY

# read go-version file unless EKSD_GO_IMAGE_TAG & GO_VERSION are set
GO_VERSION="${GO_VERSION:-"$(cat .go-version)"}"
EKSD_GO_IMAGE_TAG="${EKSD_GO_IMAGE_TAG:-"${GO_VERSION}"}"
GO_IMAGE=public.ecr.aws/eks-distro-build-tooling/golang:$EKSD_GO_IMAGE_TAG-gcc

# gotoolchain
# https://go.dev/doc/toolchain
export GOSUMDB="sum.golang.org"
export GOTOOLCHAIN=go${GO_VERSION}

# force go modules
export GO111MODULE=on

echo $GO_IMAGE
