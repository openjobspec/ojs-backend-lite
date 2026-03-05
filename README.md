# ojs-backend-lite

The lightweight, zero-dependency OJS backend for local development and testing. No Docker, no Redis, no PostgreSQL — just one binary.

## Quickstart

```bash
make run
```

That's it. The server starts in <50ms with:
- HTTP API on `:8080`
- gRPC API on `:9090`
- Admin UI at `http://localhost:8080/ojs/admin/`

## Features

- **Zero config** — works out of the box
- **Full OJS conformance** — all 5 levels (0-4)
- **In-memory storage** — instant operations, with optional SQLite persistence
- **Both HTTP and gRPC** transports
- **Embedded Admin UI** — same dashboard as production backends
- **Real-time events** — SSE and WebSocket support
- **Background scheduling** — cron jobs, retry backoff, stalled job recovery
- **Auto-retention** — terminal jobs automatically purged after configurable TTL

## Persistence

By default, all data is in-memory (ephemeral). To persist state across restarts:

```bash
OJS_PERSIST=./ojs-lite.db make run
```

This stores all jobs, queues, cron registrations, and workflows in a SQLite file. Pure Go — no CGO or external libraries required.

## Configuration

All optional, via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `OJS_PORT` | `8080` | HTTP server port |
| `OJS_GRPC_PORT` | `9090` | gRPC server port |
| `OJS_API_KEY` | _(none)_ | Bearer token for API authentication |
| `OJS_ALLOW_INSECURE_NO_AUTH` | `true` | Allow running without authentication |
| `OJS_PERSIST` | _(none)_ | SQLite file path for persistence |
| `OJS_RETENTION_COMPLETED` | `1h` | TTL for completed jobs |
| `OJS_RETENTION_CANCELLED` | `24h` | TTL for cancelled jobs |
| `OJS_RETENTION_DISCARDED` | `24h` | TTL for discarded jobs |
| `OJS_READ_TIMEOUT` | `30s` | HTTP read timeout |
| `OJS_WRITE_TIMEOUT` | `30s` | HTTP write timeout |
| `OJS_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |

## Build

```bash
make build    # → bin/ojs-server
make test     # go test ./... -race -cover
make lint     # go vet ./...
```

## When to Use

**Use ojs-backend-lite for:**
- Local development
- CI/CD test pipelines
- Learning and prototyping
- Integration testing

**Use ojs-backend-redis/postgres for:**
- Production deployments
- Multi-node clusters
- Data persistence requirements

## License

Apache 2.0




