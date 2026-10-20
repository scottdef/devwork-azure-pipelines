# syntax=docker/dockerfile:1
FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tmplforge ./cmd/tmplforge

FROM scratch
COPY --from=build /out/tmplforge /tmplforge
EXPOSE 8088
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.source="https://github.com/CoolGitOrg/tmplforge"
USER 65534:65534
ENTRYPOINT ["/tmplforge", "serve", "-a", "0.0.0.0:8088"]
