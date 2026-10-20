# syntax=docker/dockerfile:1
# Static tmplwatch on scratch. Build: make docker, or
#   docker build --build-arg VERSION=v0.1.0 -t tmplwatch .
# Run (dir source, config and templates mounted read-only):
#   docker run --rm -p 127.0.0.1:9180:9180 \
#     -v $PWD/config:/etc/tmplwatch:ro -v $PWD/templates:/templates:ro \
#     -v $PWD/testdata:/testdata:ro -v tmplwatch-out:/out \
#     tmplwatch -c /etc/tmplwatch/container.json

FROM golang:1.24 AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-mod=mod GOPROXY=off
COPY go.mod ./
COPY . .
ARG VERSION=dev
RUN go test ./internal/... && \
    for b in tmplwatch tpl; do \
      go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/$b ./cmd/$b || exit 1; \
    done && \
    mkdir -p /out/rootfs/out /out/rootfs/tmp && chown -R 65532:65532 /out/rootfs/out /out/rootfs/tmp

FROM scratch
# CA roots for api.github.com; no shell, no package manager, nothing else.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/tmplwatch /out/tpl /usr/local/bin/
COPY --from=build --chown=65532:65532 /out/rootfs/ /
USER 65532:65532
EXPOSE 9180
# -healthcheck GETs /healthz on the default listener; scratch has no curl.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD ["/usr/local/bin/tmplwatch", "-healthcheck", "-a", "127.0.0.1:9180"]
ENTRYPOINT ["/usr/local/bin/tmplwatch"]
CMD ["-c", "/etc/tmplwatch/tmplwatch.json"]
