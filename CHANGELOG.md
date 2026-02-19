# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Prometheus-compatible metrics endpoint at `/metrics`
- Detailed health check endpoint at `/ojs/v1/health/detail` with runtime, memory, and goroutine stats
- Performance benchmarks for Push, Fetch, Ack, and Nack operations
- GoReleaser configuration for automated multi-platform binary releases
- GitHub issue templates (bug report, feature request) and PR template
- Pre-commit hook script with gofmt, go vet, and test checks
- Docker Compose example for quick deployment
- CodeQL security scanning workflow
- This CHANGELOG

## [0.1.0] - 2025-01-01

### Added
- Initial release of ojs-backend-lite
- Full OJS conformance level 4 implementation
- In-memory backend with optional SQLite persistence
- HTTP API on port 8080 with all OJS v1 endpoints
- gRPC API on port 9090
- Embedded Admin UI dashboard
- Job lifecycle management (create, fetch, ack, nack, cancel)
- Priority queues with pause/resume support
- Batch job enqueue
- Dead letter queue with retry
- Cron job scheduling
- Workflow orchestration (chain and fan-out patterns)
- Real-time events via SSE and WebSocket
- Worker management with heartbeat and visibility timeout
- Stalled job recovery
- Auto-retention for terminal jobs with configurable TTL
- API key authentication with constant-time comparison
- Request body size limits (1 MB)
- CI pipeline with linting, testing, and race detection
