# Component Manager Configuration

This document explains the configuration files for the Component Manager system.

## Overview

The Component Manager configuration controls:
1. Which implementation to use for each component type (compute, NVL switch, power shelf)
2. Which API providers to enable and their settings

Timing parameters for power control and firmware update operations are configured
**per-rule** via action parameters in operation rules, not in the component manager config.

## Configuration Files

| File | Purpose |
|------|---------|
| `componentmanager.test.yaml` | Testing/development configuration using mock implementations |
| *(embedded)* | Production configuration embedded in the binary via `DefaultProdConfig()` |

The production config is compiled into the binary. No YAML file is needed for production
deployments. A YAML file is only required when overriding defaults (e.g., for testing).

## Configuration Structure

### Component Managers

```yaml
component_managers:
  compute: <implementation>
  nvlswitch: <implementation>
  powershelf: <implementation>
```

Maps each component type to its implementation. Available implementations:

| Component Type | Available Implementations | Description |
|----------------|---------------------------|-------------|
| `compute` | `nico`, `mock` | Manages compute nodes |
| `nvlswitch` | `nico`, `mock` | Manages NVLink switches |
| `powershelf` | `psm`, `mock` | Manages power shelves |

### Providers

```yaml
providers:
  nico:
    timeout: "<duration>"
    compute_power_delay: "<duration>"
  psm:
    timeout: "<duration>"
```

Configures API client providers. **A provider is enabled if its section is present** in the configuration.

| Provider | Used By | Description |
|----------|---------|-------------|
| `nico` | compute, nvlswitch | NICo API for machine management |
| `psm` | powershelf | Power Shelf Manager API |

#### Provider Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `timeout` | duration string | `1m` (nico), `30s` (psm) | gRPC call timeout |
| `compute_power_delay` | duration string | `2s` (nico only) | Delay between sequential power control calls for compute trays. Prevents overwhelming the power delivery system. Set to `0s` to disable. |

Duration strings use Go format: `30s`, `1m`, `2m30s`, etc.

## Examples

### Production Configuration (embedded default)

```go
// Equivalent to DefaultProdConfig() in internal/task/componentmanager/config.go
component_managers:
  compute: nico
  nvlswitch: nico
  powershelf: psm

providers:
  nico:
    timeout: "1m"
  psm:
    timeout: "30s"
```

### Test Configuration

```yaml
# Uses mock implementations - no external dependencies
component_managers:
  compute: mock
  nvlswitch: mock
  powershelf: mock

# No providers section needed for mock implementations
```

### Mixed Configuration (e.g., partial testing)

```yaml
# Real power shelf management, mock compute/nvlswitch
component_managers:
  compute: mock
  nvlswitch: mock
  powershelf: psm

providers:
  psm:
    timeout: "30s"
```

## Provider Auto-Detection

If the `providers` section is omitted entirely, providers are automatically enabled based on the component manager implementations:

- If any component uses `nico` → NICo provider is enabled with defaults
- If any component uses `psm` → PSM provider is enabled with defaults

This allows minimal configuration:

```yaml
component_managers:
  compute: nico
  nvlswitch: nico
  powershelf: psm
# Providers auto-enabled based on implementations above
```

## Usage

Set the configuration file path via:

1. **Command line flag**: `--component-config <path>`
2. **Environment variable**: `COMPONENT_MANAGER_CONFIG=<path>`
3. **Default**: embedded production config (nico + psm)

## Timing Parameters

Power control and firmware update timing (delays, poll intervals, timeouts) are
configured **per-rule** via action parameters in operation rules, not here.

See `CLAUDE.md` (Action-Based Operation Rules section) and
`examples/operation-rules-example.yaml` for examples.
