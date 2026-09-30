# Build context: apps/services (root modul Go).
# docker build -f chaos/Dockerfile .
#
# CATATAN KEAMANAN (ADR D19): service ini memakai /var/run/docker.sock via
# mount compose — karena itu berjalan sebagai root (akses socket), SATU-
# SATUNYA service lastmile yang begitu. Serangan tetap terbatas: kill hanya
# lolos untuk allowlist eksplisit 7 container stateless lastmile-* (validasi
# di kode, tanpa wildcard), exec berisi perintah fixed `kill -9 1`, tidak
# ada publish port host.
# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOMAXPROCS=2 go build -trimpath -ldflags="-s -w" -o /out/chaos ./chaos

FROM alpine:3.20
COPY --from=build /out/chaos /app/chaos
# root disengaja: butuh akses /var/run/docker.sock (lihat ADR D19)
ENV PORT=4206 DOCKER_SOCK=/var/run/docker.sock
EXPOSE 4206
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=5 \
  CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/healthz" || exit 1
ENTRYPOINT ["/app/chaos"]
