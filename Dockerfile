# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/umc-auth ./cmd/umc-auth

FROM alpine:3.21
RUN addgroup -g 10001 umc \
    && adduser -D -u 10001 -G umc -h /data umc \
    && mkdir -p /data \
    && chown 10001:10001 /data \
    && apk add --no-cache ca-certificates
COPY --from=builder /out/umc-auth /usr/local/bin/umc-auth
COPY LICENSE /usr/share/doc/umc-auth/LICENSE
USER 10001
ENV BIND_ADDR=0.0.0.0:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["umc-auth"]
