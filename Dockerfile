ARG golang_image=public.ecr.aws/eks-distro-build-tooling/golang:1.22

FROM --platform=$BUILDPLATFORM $golang_image AS builder
WORKDIR $GOPATH/src/github.com/aws/amazon-eks-pod-identity-webhook
COPY . ./
RUN go version
ARG TARGETOS TARGETARCH
RUN GOPROXY=direct CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /webhook -v -a -ldflags="-buildid='' -w -s" .

FROM --platform=$TARGETPLATFORM public.ecr.aws/eks-distro/kubernetes/go-runner:v0.16.4-eks-1-30-latest
COPY --from=builder /webhook /webhook
ENTRYPOINT ["/go-runner"]
CMD ["/webhook"]
