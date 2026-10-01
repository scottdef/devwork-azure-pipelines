# Image for the AKS Job in deploy/k8s. Multi-stage: golang:1.21 builds a
# static binary; the runtime is Alpine with git and git-lfs so the repos and
# wiki components can mirror Git content in-cluster. Runs as uid 65532 to match
# the Job's securityContext.
#
#   docker build --build-arg VERSION=1.1.0 -t <acr>.azurecr.io/adoclone:1.1.0 .
#   az acr login -n <acr> && docker push <acr>.azurecr.io/adoclone:1.1.0
FROM golang:1.21 AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test ./... \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/adoclone ./cmd/adoclone

FROM alpine:3.20
RUN apk add --no-cache git git-lfs ca-certificates \
 && adduser -D -H -u 65532 -s /sbin/nologin adoclone
COPY --from=build /out/adoclone /usr/local/bin/adoclone
USER 65532:65532
# git needs a writable HOME; the Job mounts an emptyDir at /tmp.
ENV HOME=/tmp
ENTRYPOINT ["/usr/local/bin/adoclone"]
