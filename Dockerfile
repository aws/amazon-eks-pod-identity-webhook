FROM golang AS builder

WORKDIR $GOPATH/src/github.com/aws/amazon-eks-pod-identity-webhook
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix nocgo -o /webhook .

FROM scratch
COPY --from=builder /webhook /webhook
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
EXPOSE 443
VOLUME /etc/webhook
ENTRYPOINT ["/webhook"]
CMD ["--logtostderr"]
