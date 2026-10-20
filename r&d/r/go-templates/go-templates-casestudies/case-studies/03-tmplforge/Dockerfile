# syntax=docker/dockerfile:1
# tmplforge: static binary on scratch. Builds offline: the only dependency
# (go-echarts) is vendored under third_party/ and consumed via replace.
ARG GO_VERSION=1.24
FROM golang:${GO_VERSION} AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-mod=mod GOPROXY=off
COPY . .
ARG VERSION=dev
RUN go test ./internal/forge -run 'Catalog|Render|Lint|Vars|Diff' \
 && go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/tmplforge ./cmd/tmplforge \
 && mkdir -p /out/tmp && chmod 1777 /out/tmp

FROM scratch
# The sandbox re-executes /tmplforge and hands fixtures over in $TMPDIR,
# so the image needs a writable /tmp (mount a tmpfs there when running
# with a read-only root filesystem).
COPY --from=build /out/tmp /tmp
COPY --from=build /out/tmplforge /tmplforge
COPY LICENSE /LICENSE
USER 65534:65534
EXPOSE 8088
ENV TMPDIR=/tmp
# Off loopback a token is mandatory: pass TMPLFORGE_TOKEN at run time.
ENTRYPOINT ["/tmplforge"]
CMD ["serve", "-a", "0.0.0.0:8088"]
