FROM golang:1.26-alpine AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go test -v ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o k8s-athenz-syncer .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /
COPY --from=builder /workspace/k8s-athenz-syncer /usr/bin/k8s-athenz-syncer

# TODO: Consider using a non-root user for better security practices
# Run as a non-root user for security best practices
# USER 65532:65532

ENTRYPOINT ["/usr/bin/k8s-athenz-syncer"]
