# AGENTS.md

This file provides guidance for AI coding agents working in the
`infra-controller-rest` repository.

## Project Overview

**NVIDIA Infra Controller REST** is a collection of Go microservices that comprise
the management backend for NVIDIA Infra Controller, exposed as a REST API. It
provides multi-tenant, API-driven bare-metal lifecycle management, working in
concert with [NVIDIA Infra Controller Core](https://github.com/NVIDIA/infra-controller-core)
for on-site hardware operations.

> **Status:** Experimental/Preview. APIs, configurations, and features may
> change without notice between releases.

### Key Responsibilities

- REST API for hardware inventory, provisioning, and lifecycle orchestration
- Multi-tenant site and instance management
- Temporal-based cloud and site workflow orchestration
- On-site agent for datacenter-local operations
- IP address management (IPAM)
- Authentication and authorization (Keycloak, JWT, service accounts)
- Native PKI certificate management
- CLI client (`carbidecli`) with interactive TUI

## Repository Structure

```text
infra-controller-rest/
├── api/                  # Main REST API server (Echo-based)
├── auth/                 # Authentication (Keycloak, JWT, service accounts)
├── cert-manager/         # Native PKI certificate management (credsmgr)
├── cli/                  # CLI client (carbidecli) with TUI
├── common/               # Shared utilities and configuration
├── db/                   # Database layer (Bun ORM, pgx, migrations)
├── deploy/               # Kubernetes deployment (Kind, Kustomize, Helm)
├── docker/               # Dockerfiles (local dev and production)
├── helm/                 # Helm charts for Kubernetes deployment
├── ipam/                 # IP address management
├── nvswitch-manager/     # NVSwitch firmware management (NSM)
├── openapi/              # OpenAPI spec and SDK generation
├── powershelf-manager/   # Power shelf management (PSM)
├── rla/                  # Rack Level Agent (RLA) logic
├── sdk/                  # Go API client (simple and standard variants)
├── site-agent/           # On-site agent for datacenter
├── site-manager/         # Site management service (sitemgr)
├── site-workflow/        # Site-level Temporal workflows
├── temporal-helm/        # Temporal Helm chart
├── workflow/             # Cloud Temporal workflows and activities
├── workflow-schema/      # Protobuf and workflow schemas
├── .github/              # GitHub Actions workflows and templates
├── Makefile              # Primary build/task automation
└── go.mod                # Go module and dependency management
```

## Technology Stack

- **Language:** Go (version specified in `go.mod`; module `github.com/NVIDIA/ncx-infra-controller-rest`)
- **HTTP framework:** Echo v4 (with middleware for CORS, auth, rate limiting, audit)
- **Database:** PostgreSQL via pgx v5 (connection pool) and Bun ORM (queries, migrations)
- **Workflow engine:** Temporal (cloud and site workflows/activities)
- **gRPC:** Connect-RPC and google.golang.org/grpc (site-agent, workflow schemas)
- **Protobuf:** buf for code generation
- **Observability:** OpenTelemetry, Prometheus (echoprometheus), Sentry
- **Auth:** Keycloak, JWT
- **Testing:** testify (assert/require/suite), go-sqlmock, testcontainers-go, gomock
- **Build tool:** Make

## Build, Test, and Lint Commands

### Building

```bash
# Build all binaries (linux/amd64, static)
make build

# Build and install CLI to $GOPATH/bin
make carbide-cli

# Build Docker images (production)
make docker-build

# Build Docker images (local dev, public base images)
make docker-build-local
```

### Testing

```bash
# Run all tests (auto-manages PostgreSQL container)
make test

# Module-level tests
make test-api
make test-db
make test-workflow
make test-auth
make test-common
make test-cert-manager
make test-site-agent        # requires mock gRPC servers
make test-site-manager
make test-site-workflow
make test-ipam

# PostgreSQL management for tests
make postgres-up            # start test PostgreSQL container
make postgres-down          # stop test PostgreSQL container
make ensure-postgres        # start if not running, wait until ready
make migrate                # run database migrations against test DB
```

Tests require a PostgreSQL container (postgres:14.4-alpine) on port 30432.
The Makefile manages this automatically via `ensure-postgres`.

### Linting and Formatting

```bash
# Check formatting (fails if repo is dirty after go fmt)
make fmt-go

# Run all linters (go vet + golangci-lint + revive)
make lint-go

# Auto-fix formatting
go fmt ./...
```

### OpenAPI

```bash
# Lint the OpenAPI spec
make lint-openapi

# Preview in Redoc UI (http://127.0.0.1:8090)
make preview-openapi

# Generate Go SDK from OpenAPI spec
make generate-sdk

# Publish OpenAPI docs
make publish-openapi
```

### Protobuf Code Generation

```bash
make carbide-proto          # fetch proto files from carbide-core
make carbide-protogen       # generate Go code from protos
make rla-proto              # fetch RLA proto files
make rla-protogen           # generate Go code from RLA protos
```

### Local Development (Kind cluster)

```bash
make kind-reset             # full reset: cluster + infra + Helm deploy
make kind-reset-kustomize   # full reset with Kustomize instead of Helm
make kind-redeploy          # rebuild and restart (fast iteration)
make helm-redeploy          # rebuild and restart via Helm
make kind-status            # check pod status
make kind-logs              # tail API logs
make kind-verify            # health checks
make kind-down              # tear down cluster
```

## Coding Conventions

- Follow standard Go conventions; `go fmt` is enforced in CI.
- Linting uses `golangci-lint` (v2 config in `.golangci.yml`) with most
  linters enabled, plus `revive` (config in `.revive.toml`).
- Use `testify` (assert/require) for test assertions.
- Tests that need a database use a PostgreSQL container (testcontainers-go
  or the Makefile-managed container).
- Tests run with `-p 1` (serial) and often with `-race`.
- API handlers live in `api/pkg/api/handler/`, request/response models in
  `api/pkg/api/model/`, and DB models in `db/pkg/db/model/`.
- OpenAPI schema in `openapi/spec.yaml` must be updated whenever API
  endpoints are added or modified.

## Git Workflow

When writing git commit messages, follow the conventions below:

- Use `git mv` to move files already checked into git.
- Explain non-obvious trade-offs in the commit message.
- Wrap prose (not code) to match git commit conventions; follow semantic
  commit conventions for the title (e.g. `feat:`, `fix:`, `chore:`).
- Use backticks for types or short code snippets; use indented code blocks
  for full lines of code.

## Code Style Preferences

- Document when you have intentionally omitted code that the reader might
  otherwise expect to be present.
- Add TODO comments for features or nuances not important to implement
  right away.

## Commit Guidelines

All commits **must** meet the following signing requirement:

- **DCO sign-off** — certifies the Developer Certificate of Origin:
  ```bash
  git commit -s -m "Your commit message"
  ```
  DCO compliance is enforced automatically; unsigned commits block merging.

## Pull Request Guidelines

- Write PR descriptions as if the audience has no context: explain the *why*.
- Reference related issues.
- Keep PRs focused on a single change.
- Do not land unused code unless the PR is too large to review otherwise.
- Ensure all CI checks pass before requesting review.

## CI / CD

The primary CI workflow (`.github/workflows/main-build.yml`) runs on pushes
to `main`, `feat/**`, `fix/**`, `chore/**`, `hotfix/**`, `version/**`,
and `pull-request/[0-9]+` branches, as well as `v*.*.*` tags and manual
`workflow_dispatch`. It performs:

- Style checks (`go fmt`, `revive`, `go vet`)
- Lint (`golangci-lint`)
- OpenAPI spec validation
- Generated files check
- Test matrix across all modules (with PostgreSQL service container)
- Binary builds (api, workflow, migrations, sitemgr, credsmgr, site-agent)
- Security scanning (TruffleHog)
- Docker image builds and pushes
- Helm chart validation
- Release promotion

## Pre-commit Hooks

```bash
make pre-commit-install     # install pre-commit + trufflehog hooks
make pre-commit-run         # scan all files for secrets
make pre-commit-update      # update hooks to latest versions
```

## Further Reading

- [`README.md`](README.md) — Project overview and getting started
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — Contribution workflow and DCO process
- [`openapi/README.md`](openapi/README.md) — OpenAPI schema development
- [`cli/README.md`](cli/README.md) — CLI client reference
- [`deploy/README.md`](deploy/README.md) — Deployment quickstart guide
- [`deploy/INSTALLATION.md`](deploy/INSTALLATION.md) — Detailed installation guide
