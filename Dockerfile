FROM golang:1.14 AS builder

WORKDIR $GOPATH/src/github.com/aws/amazon-eks-pod-identity-webhook
COPY . ./
ARG goproxy=https://goproxy.cn,direct
ENV GOPROXY=$goproxy

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -v -a -installsuffix nocgo -o /webhook .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /webhook /webhook
EXPOSE 443
VOLUME /etc/webhook
ENTRYPOINT ["/webhook"]
CMD ["--logtostderr"]
