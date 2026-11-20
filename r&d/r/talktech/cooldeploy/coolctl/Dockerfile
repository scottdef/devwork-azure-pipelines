# One build, one image per binary:
#
#   az acr build -r prdeus2coolopsacr001 -t platform/cool-api:1.0.0    --target cool-api    .
#   az acr build -r prdeus2coolopsacr001 -t platform/coolctl:1.0.0     --target coolctl     .
#   az acr build -r prdeus2coolopsacr001 -t platform/cool-static:1.0.0 --target cool-static .
#   az acr build -r prdeus2coolopsacr001 -t sim/appsim:1.0.0           --target appsim      .
#
# Base images come from the platform registry's mirror (cool-infra imports
# them), so a build never reaches a public registry.
ARG REGISTRY=prdeus2coolopsacr001.azurecr.io

FROM ${REGISTRY}/base/golang:1.24 AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/ ./cmd/...

FROM ${REGISTRY}/base/distroless-static:nonroot AS cool-api
COPY --from=build /out/cool-api /cool-api
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/cool-api"]

FROM ${REGISTRY}/base/distroless-static:nonroot AS coolctl
COPY --from=build /out/coolctl /coolctl
USER 65532:65532
ENTRYPOINT ["/coolctl"]

FROM ${REGISTRY}/base/distroless-static:nonroot AS cool-static
COPY --from=build /out/cool-static /cool-static
USER 65532:65532
ENV PORT=8080 COOL_STATIC_DIR=/site
EXPOSE 8080
ENTRYPOINT ["/cool-static"]

FROM ${REGISTRY}/base/distroless-static:nonroot AS appsim
COPY --from=build /out/appsim /appsim
USER 65532:65532
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/appsim"]
