FROM golang:1.14.7 as builder

WORKDIR $GOPATH/src/github.com/AthenZ/k8s-athenz-syncer

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go install ./... && \
    go test ./...

FROM alpine:latest

RUN apk --update add ca-certificates

COPY --from=builder /go/bin/k8s-athenz-syncer /usr/bin/k8s-athenz-syncer

ENTRYPOINT ["/usr/bin/k8s-athenz-syncer"]
