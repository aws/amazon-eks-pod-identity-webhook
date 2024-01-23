#!/usr/bin/env bash
set -euo pipefail

source hack/setup-go.sh

go version
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
