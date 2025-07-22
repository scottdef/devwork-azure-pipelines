# Multi-stage Dockerfile for Azure DevOps CLI
# Build stage
FROM golang:1.21-alpine AS builder

# Set build information
ARG BUILD_DATE
ARG BUILD_VERSION=1.0.0
ARG BUILD_COMMIT=unknown

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Create app directory
WORKDIR /app

# Copy go source
COPY ado-cli.go .

# Build the application with optimization flags
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a -installsuffix cgo \
    -ldflags="-w -s -X main.BuildDate=${BUILD_DATE} -X main.BuildVersion=${BUILD_VERSION} -X main.BuildCommit=${BUILD_COMMIT}" \
    -o ado-cli \
    ado-cli.go

# Verify the binary
RUN chmod +x ado-cli && ./ado-cli help

# Runtime stage
FROM alpine:3.18

# Install runtime dependencies
RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    jq \
    curl \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 -S ado && \
    adduser -u 1000 -S ado -G ado

# Create directories
RUN mkdir -p /app /data && \
    chown -R ado:ado /app /data

# Copy binary from builder stage
COPY --from=builder /app/ado-cli /app/ado-cli

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Set working directory
WORKDIR /data

# Switch to non-root user
USER ado

# Set environment variables
ENV PATH="/app:${PATH}"
ENV TZ=UTC

# Add labels
LABEL maintainer="DevOps Team" \
      version="${BUILD_VERSION}" \
      description="Azure DevOps CLI for environment and deployment management" \
      org.opencontainers.image.title="ADO CLI" \
      org.opencontainers.image.description="Azure DevOps CLI tool for managing environments and analyzing deployments" \
      org.opencontainers.image.version="${BUILD_VERSION}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${BUILD_COMMIT}" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.source="https://github.com/your-org/ado-cli"

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ado-cli help > /dev/null || exit 1

# Default command
ENTRYPOINT ["ado-cli"]
CMD ["help"]
