# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o ojs-server ./cmd/ojs-server

# Final stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 1001 ojs

LABEL org.opencontainers.image.source="https://github.com/openjobspec/ojs-backend-lite" \
      org.opencontainers.image.description="OJS Lite Backend (in-memory, zero dependencies)" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=builder /build/ojs-server /usr/local/bin/ojs-server
USER ojs
EXPOSE 8080 9090

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/ojs/v1/health || exit 1

ENV OJS_ALLOW_INSECURE_NO_AUTH=false

ENTRYPOINT ["ojs-server"]
