# syntax=docker/dockerfile:1
# agent-runner: Go 1.21, standard library only, static binary on distroless.
# Build from the repository root:
#   docker build -f deploy/agent-runner/Dockerfile --build-arg VERSION=1.0.0 -t agent-runner:1.0.0 .

FROM golang:1.21.13-bookworm AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=local GOFLAGS=-mod=readonly
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG VERSION=dev
RUN go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o /out/agent-runner ./cmd/agent-runner

# distroless/static: no shell, no package manager; CA certificates and tzdata only.
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/CoolEngOrg/issueops" \
      org.opencontainers.image.title="issueops-agent-runner" \
      org.opencontainers.image.description="Bounded plan/draft/critique agent loop against Microsoft Foundry (workload identity)"
COPY --from=build /out/agent-runner /agent-runner
USER 65532:65532
ENTRYPOINT ["/agent-runner"]
