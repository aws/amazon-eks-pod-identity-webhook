ARG BUILDER=public.ecr.aws/bitnami/golang:latest
ARG BASE_IMAGE=public.ecr.aws/eks-distro/kubernetes/go-runner:v0.13.0-eks-1-23-1

FROM public.ecr.aws/eks-distro/kubernetes/go-runner:v0.13.0-eks-1-23-1 as go-runner

FROM ${BUILDER} AS builder
WORKDIR $GOPATH/src/github.com/aws/amazon-eks-pod-identity-webhook
COPY . ./
RUN GOPROXY=direct CGO_ENABLED=0 GOOS=linux go build -o /webhook -v -a -installsuffix nocgo -ldflags="-buildid='' -w -s" .

FROM ${BASE_IMAGE}
COPY ATTRIBUTIONS.txt /ATTRIBUTIONS.txt
COPY --from=builder /webhook /webhook
EXPOSE 443
VOLUME /etc/webhook
ENTRYPOINT ["/webhook"]
CMD ["--logtostderr"]
