<!--
SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
SPDX-License-Identifier: Apache-2.0
-->

# NICo CLI

Command-line client for the NVIDIA Infrastructure Controller (NICo) REST API. Commands are dynamically generated from the embedded OpenAPI spec at startup, so every API endpoint is available with zero manual command code.

## Prerequisites

- Go 1.25.4 or later
- Access to a running NVIDIA Infrastructure Controller (NICo) REST API instance (local via `make kind-reset` or remote)

## Installation

### From the repo (recommended)

```bash
make nico-cli
```

This builds and installs `cli` to `$(go env GOPATH)/bin/cli`. Override the destination with:

```bash
make nico-cli INSTALL_DIR=/usr/local/bin
```

### With go install

```bash
go install ./cli/cmd/cli
```

### Manual go build

```bash
go build -o /usr/local/bin/cli ./cli/cmd/cli
```

### Verify

```bash
cli --version
```

## Quick Start

Generate a default config and add configs for each environment you work with:

```bash
cli init                    # writes ~/.nico/config.yaml
cp ~/.nico/config.yaml ~/.nico/config.staging.yaml
cp ~/.nico/config.yaml ~/.nico/config.prod.yaml
```

Edit each file with the appropriate server URL, org, and auth settings for that environment (see Configuration below), then launch interactive mode:

```bash
cli tui
```

The TUI will list your configs, let you pick an environment, authenticate, and start running commands. This is the recommended way to use `cli` since it handles environment selection, login, and token refresh automatically.

For direct one-off commands without the TUI:

```bash
cli login                   # exchange credentials for a token
cli site list               # list all sites
```

## Configuration

Config file: `~/.nico/config.yaml`

```yaml
api:
  base: http://localhost:8388
  org: test-org
  name: nico                # API path segment (default)

auth:
  # Option 1: Direct bearer token
  # token: eyJhbGciOi...

  # Option 2: OIDC provider (e.g. Keycloak)
  oidc:
    token_url: http://localhost:8080/realms/nico-dev/protocol/openid-connect/token
    client_id: nico-api
    client_secret: nico-local-secret

  # Option 3: NGC API key
  # api_key:
  #   authn_url: https://authn.nvidia.com/token
  #   key: nvapi-xxxx
```

Flags and environment variables override config values:

| Flag | Env Var | Description |
|------|---------|-------------|
| `--base-url` | `NICO_BASE_URL` | API base URL |
| `--org` | `NICO_ORG` | Organization name |
| `--token` | `NICO_TOKEN` | Bearer token |
| `--token-url` | `NICO_TOKEN_URL` | OIDC token endpoint URL |
| `--keycloak-url` | `NICO_KEYCLOAK_URL` | Keycloak base URL (constructs token-url) |
| `--keycloak-realm` | `NICO_KEYCLOAK_REALM` | Keycloak realm (default: `nico-dev`) |
| `--client-id` | `NICO_CLIENT_ID` | OAuth client ID |
| `--output`, `-o` | | Output format: `json` (default), `yaml`, `table` |

## Authentication

```bash
# OIDC (credentials from config, prompts for password if not stored)
cli login

# OIDC with explicit flags
cli --token-url https://auth.example.com/token login --username admin@example.com

# NGC API key
cli login --api-key nvapi-xxxx

# Keycloak shorthand
cli --keycloak-url http://localhost:8080 login --username admin@example.com
```

Tokens are saved to `~/.nico/config.yaml` with auto-refresh for OIDC.

## Usage

```bash
cli site list
cli site get <siteId>
cli site create --name "SJC4"
cli site create --data-file site.json
cat site.json | cli site create --data-file -
cli site delete <siteId>
cli instance list --status provisioned --page-size 20
cli instance list --all                # fetch all pages
cli allocation constraint create <allocationId> --constraint-type SITE
cli site list --output table
cli --debug site list
```

## Command Structure

Commands follow `cli <resource> [sub-resource] <action> [args] [flags]`.

| Spec Pattern | CLI Action |
|---|---|
| `get-all-*` | `list` |
| `get-*` | `get` |
| `create-*` | `create` |
| `update-*` | `update` |
| `delete-*` | `delete` |
| `batch-create-*` | `batch-create` |
| `get-*-status-history` | `status-history` |
| `get-*-stats` | `stats` |

Nested API paths appear as sub-resource groups:

```
cli allocation list
cli allocation constraint list
cli allocation constraint create <allocationId>
```

## Shell Completion

```bash
# Bash
eval "$(cli completion bash)"

# Zsh
eval "$(cli completion zsh)"

# Fish
cli completion fish > ~/.config/fish/completions/cli.fish
```

## Multi-Environment Configs

Each environment (local dev, staging, prod) gets its own config file in `~/.nico/`:

```
~/.nico/config.yaml           # default (local dev)
~/.nico/config.staging.yaml   # staging
~/.nico/config.prod.yaml      # production
```

The TUI automatically discovers all `config*.yaml` files in `~/.nico/` and presents them as a selection list at startup. This is the easiest way to switch between environments without remembering URLs or re-authenticating.

For direct commands, select an environment with `--config`:

```bash
cli --config ~/.nico/config.staging.yaml site list
```

## Interactive TUI Mode

The TUI is the recommended way to interact with the API. It handles config selection, authentication, and token refresh in one session:

```bash
cli tui
```

You can also launch it with the `i` alias:

```bash
cli i
```

To skip the config selector and connect to a specific environment directly:

```bash
cli --config ~/.nico/config.prod.yaml tui
```

## Troubleshooting

If `cli` is not found after install, make sure `$(go env GOPATH)/bin` is in your PATH:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

Use `--debug` on any command to see the full HTTP request and response for diagnosing issues:

```bash
cli --debug site list
```
