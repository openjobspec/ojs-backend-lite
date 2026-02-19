# Contributing to ojs-backend-lite

Thank you for considering contributing to ojs-backend-lite! This guide will help you get started.

## Development Setup

### Prerequisites

- Go 1.24+
- Make

### Getting Started

```bash
# Clone the repository
git clone https://github.com/openjobspec/ojs-backend-lite.git
cd ojs-backend-lite

# Build
make build

# Run tests
make test

# Run linter
make lint

# Run the server locally
make run
```

The server starts with:
- HTTP API on `:8080`
- gRPC API on `:9090`
- Admin UI at `http://localhost:8080/ojs/admin/`

### Sibling Repositories

This project depends on sibling repos via `replace` directives in `go.mod`:
- `ojs-go-backend-common` — shared types and utilities
- `ojs-proto` — gRPC protobuf definitions

Clone them alongside this repo for local development.

## Making Changes

### Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Use `golangci-lint` if available (see `.golangci.yml`)
- Comment only where the logic is non-obvious
- Keep functions focused and small

### Testing

- Write tests for new functionality
- Run `make test` before submitting (includes `-race` flag)
- Aim for meaningful test coverage, not just line coverage

### Commit Messages

- Use clear, descriptive commit messages
- Reference issues when applicable (e.g., `Fixes #42`)

## Pull Request Process

1. Fork the repository and create a feature branch
2. Make your changes with appropriate tests
3. Ensure `make test` and `make lint` pass
4. Submit a pull request with a clear description of the changes
5. Wait for review — maintainers will provide feedback

## Reporting Issues

- Use GitHub Issues to report bugs or request features
- Include steps to reproduce for bug reports
- Check existing issues before creating a new one

## License

By contributing, you agree that your contributions will be licensed under the Apache 2.0 License.
