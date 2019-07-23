FROM golang:1.12.4 as builder

WORKDIR $GOPATH/src/github.com/yahoo/k8s-athenz-syncer

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go install ./... && \
    go test ./...

FROM alpine:latest

RUN apk --update add ca-certificates

COPY --from=builder /go/bin/k8s-athenz-syncer /usr/bin/k8s-athenz-syncer

CMD ["/usr/bin/k8s-athenz-syncer"]
