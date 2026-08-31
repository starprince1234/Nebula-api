FROM docker.m.daocloud.io/library/golang:1.26.6-alpine3.24 AS builder
ENV GOPROXY=https://goproxy.cn,direct
ENV GOMAXPROCS=1
ENV GOFLAGS=-p=1
ENV GOMODCACHE=/go/pkg/mod
ENV GOCACHE=/root/.cache/go-build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,id=nebula-go-mod,target=/go/pkg/mod,sharing=locked \
    go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG VERSION
RUN --mount=type=cache,id=nebula-go-mod,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=nebula-go-build,target=/root/.cache/go-build,sharing=locked \
    test -n "$VERSION" && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/nebula-server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/nebula-migrate ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/nebula-maintenance ./cmd/maintenance
RUN --mount=type=cache,id=nebula-go-mod,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=nebula-go-build,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/nebula-model-catalog ./cmd/modelcatalog

FROM docker.m.daocloud.io/library/alpine:3.22.5 AS runtime
WORKDIR /app
COPY --from=builder --chown=10001:10001 /out/nebula-server /app/nebula-server
COPY --from=builder --chown=10001:10001 /out/nebula-migrate /app/nebula-migrate
COPY --from=builder --chown=10001:10001 /out/nebula-maintenance /app/nebula-maintenance
COPY --from=builder --chown=10001:10001 /out/nebula-model-catalog /app/nebula-model-catalog

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S -g 10001 nebula && \
    adduser -S -D -H -u 10001 -G nebula nebula

ARG VERSION
LABEL org.opencontainers.image.title="Nebula API" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.source="https://github.com/starprince1234/Nebula-api"

USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=5 \
  CMD wget -q -T 2 -O /dev/null http://127.0.0.1:8080/health/ready || exit 1

ENTRYPOINT ["/app/nebula-server"]
