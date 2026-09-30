# Build context: apps/services (root modul Go).
# docker build -f strategy-lab/Dockerfile .
# Graph Berlin disalin dari rider-sim (satu sumber kebenaran, tanpa duplikasi repo).
# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOMAXPROCS=2 go build -trimpath -ldflags="-s -w" -o /out/strategy-lab ./strategy-lab

FROM alpine:3.20
RUN addgroup -g 10001 app && adduser -D -u 10001 -G app app
COPY --from=build /out/strategy-lab /app/strategy-lab
COPY rider-sim/data/berlin_graph.json /app/data/berlin_graph.json
USER app
ENV PORT=4205 GRAPH_PATH=/app/data/berlin_graph.json
EXPOSE 4205
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=5 \
  CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/healthz" || exit 1
ENTRYPOINT ["/app/strategy-lab"]
