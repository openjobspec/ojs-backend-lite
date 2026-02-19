# Docker Compose Example

Run the full OJS backend-lite stack with a single command:

```bash
docker compose up --build
```

This starts:

- **OJS Server** on `http://localhost:8080` (HTTP) and `localhost:9090` (gRPC)
- **Admin UI** at `http://localhost:8080/ojs/admin/`
- **SQLite persistence** stored in a Docker volume

## Endpoints

| Endpoint | URL |
|----------|-----|
| Health | http://localhost:8080/ojs/v1/health |
| Health Detail | http://localhost:8080/ojs/v1/health/detail |
| Manifest | http://localhost:8080/ojs/manifest |
| Admin Stats | http://localhost:8080/ojs/v1/admin/stats |
| Metrics | http://localhost:8080/metrics |
| Admin UI | http://localhost:8080/ojs/admin/ |

## Quick Test

```bash
# Create a job
curl -X POST http://localhost:8080/ojs/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"example","queue":"default","args":{"message":"hello"}}'

# Fetch a job
curl -X POST http://localhost:8080/ojs/v1/workers/fetch \
  -H "Content-Type: application/json" \
  -d '{"queues":["default"],"count":1,"worker_id":"worker-1","visibility_ms":30000}'
```

## Stopping

```bash
docker compose down        # Stop containers
docker compose down -v     # Stop and remove data volume
```
