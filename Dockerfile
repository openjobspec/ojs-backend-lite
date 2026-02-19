# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o ojs-server ./cmd/ojs-server

# Final stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /build/ojs-server /usr/local/bin/ojs-server

EXPOSE 8080 9090

ENV OJS_ALLOW_INSECURE_NO_AUTH=false

ENTRYPOINT ["ojs-server"]
