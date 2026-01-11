FROM golang:1.25-alpine AS builder

# Install essential packages (git is often required for fetching dependencies)
RUN apk add --no-cache git

WORKDIR /workspace

# Copy dependency files first to leverage Docker cache
# (Prevents re-downloading modules when only source code changes)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .

# -ldflags="-w -s": Strip debug information to reduce binary size
# -o manager: Explicitly name the output binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o manager .

FROM alpine:latest

# Install CA certificates and timezone data (Essential for logs and HTTPS)
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /
COPY --from=builder /workspace/manager .

# TODO: Consider using a non-root user for better security practices
# Run as a non-root user for security best practices
# USER 65532:65532

ENTRYPOINT ["/manager"]
