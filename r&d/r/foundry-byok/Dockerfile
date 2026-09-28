# syntax=docker/dockerfile:1
# foundry-byok: stdlib-only Go 1.21 binary on a distroless, non-root base.

FROM golang:1.21-bookworm AS build
ARG VERSION=dev
ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-mod=readonly
WORKDIR /src
# No go.sum: the module has no third-party dependencies.
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY examples/inventory.json ./examples/inventory.json
RUN go vet ./... && go test ./... \
 && go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/foundry-byok ./cmd/foundry-byok

# distroless/static ships CA certificates, which `verify` needs for TLS to Azure.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/foundry-byok /usr/local/bin/foundry-byok
USER nonroot:nonroot
WORKDIR /work
ENTRYPOINT ["/usr/local/bin/foundry-byok"]
CMD ["help"]

# Usage:
#   docker run --rm -v "$PWD:/work" foundry-byok generate --inventory inventory.json --out out
#   docker run --rm -e FOUNDRY_DEV_API_KEY foundry-byok verify --resource ... --api-key-secret FOUNDRY_DEV_API_KEY
# On Linux, add  --user "$(id -u):$(id -g)"  so generated files are owned by you.
