# Build context: apps/services (root modul Go).
# docker build -f ws-gateway/Dockerfile .
# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ws-gateway ./ws-gateway

FROM alpine:3.20
RUN addgroup -g 10001 app && adduser -D -u 10001 -G app app
COPY --from=build /out/ws-gateway /app/ws-gateway
USER app
ENV PORT=3012
EXPOSE 3012
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=5 \
  CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/healthz" || exit 1
ENTRYPOINT ["/app/ws-gateway"]
