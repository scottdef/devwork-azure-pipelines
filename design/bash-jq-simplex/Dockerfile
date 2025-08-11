# Multi-purpose Dockerfile for Kubernetes Administration
# Supports bash commands, curl, jq, and Go development
FROM ubuntu:22.04

# Avoid prompts from apt
ENV DEBIAN_FRONTEND=noninteractive

# Install system dependencies
RUN apt-get update && apt-get install -y \
    bash \
    curl \
    jq \
    wget \
    git \
    build-essential \
    ca-certificates \
    software-properties-common \
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.21
ENV GO_VERSION=1.21.12
ENV GOROOT=/usr/local/go
ENV GOPATH=/go
ENV PATH=$GOROOT/bin:$GOPATH/bin:$PATH

RUN wget https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz \
    && rm go${GO_VERSION}.linux-amd64.tar.gz

# Install kubectl 1.30
RUN curl -LO "https://dl.k8s.io/release/v1.30.0/bin/linux/amd64/kubectl" \
    && chmod +x kubectl \
    && mv kubectl /usr/local/bin/

# Create Go workspace and data directories
RUN mkdir -p $GOPATH/src $GOPATH/bin $GOPATH/pkg /data /scripts

# Set working directory
WORKDIR /app

# Create a sample Go module for go-echarts usage
RUN go mod init k8s-admin-tools

# Install allowed external Go package
RUN go get github.com/go-echarts/go-echarts/v2@latest

# Create utility scripts directory
COPY scripts/ /scripts/
RUN chmod +x /scripts/*.sh

# Create data volume mount point
VOLUME ["/data"]

# Default command
CMD ["/bin/bash"]

# Health check using curl
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Expose common ports for development/monitoring
EXPOSE 8080 3000 9090
