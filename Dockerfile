# Build stage
FROM golang:1.25-alpine AS builder

# Set working directory
WORKDIR /app

# Install git and ca-certificates for secure HTTPS connections
RUN apk add --no-cache git ca-certificates

# Copy go mod files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
# CGO_ENABLED=0 for static binary
# -ldflags to strip debug info and set version/commit
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags='-w -s -extldflags "-static"' \
  -a -installsuffix cgo \
  -o external-dns-provider-mikrotik \
  ./main.go

# Final stage - minimal runtime image
FROM gcr.io/distroless/static-debian12:nonroot

# Copy the binary from builder stage
COPY --from=builder /app/external-dns-provider-mikrotik /external-dns-provider-mikrotik

# Use non-root user for security
USER nonroot:nonroot

# Expose the default port
EXPOSE 8080

ENTRYPOINT ["/external-dns-provider-mikrotik"]