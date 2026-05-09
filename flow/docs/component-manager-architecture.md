# Component Manager Architecture

This document explains the architecture of the Component Manager system, including the Provider pattern and Factory pattern used for dependency injection and extensibility.

## Overview

The Component Manager system uses two main patterns:

1. **Provider Pattern** - Wraps API clients and manages their lifecycle
2. **Factory Pattern** - Creates component manager instances with their required dependencies

```
┌─────────────────────────────────────────────────────────────────────┐
│                         cmd/serve.go                                │
│  (Application Entry Point - Wiring & Bootstrap)                     │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      ProviderRegistry                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │
│  │   nico   │  │     psm     │  │   (new...)  │                 │
│  │  Provider   │  │  Provider   │  │  Provider   │                 │
│  └─────────────┘  └─────────────┘  └─────────────┘                 │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    ComponentManager Registry                        │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ ComponentType: Compute                                       │   │
│  │   ├── "nico" → Factory → Manager (uses nico.Provider)  │   │
│  │   └── "mock"    → Factory → Manager (no provider needed)     │   │
│  ├─────────────────────────────────────────────────────────────┤   │
│  │ ComponentType: NVLSwitch                                     │   │
│  │   ├── "nico" → Factory → Manager                          │   │
│  │   └── "mock"    → Factory → Manager                          │   │
│  ├─────────────────────────────────────────────────────────────┤   │
│  │ ComponentType: PowerShelf                                    │   │
│  │   ├── "psm"     → Factory → Manager (uses psm.Provider)      │   │
│  │   └── "mock"    → Factory → Manager                          │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

## Key Components

### Provider Interface

```go
// Provider is a marker interface for API client providers.
type Provider interface {
    Name() string  // Unique identifier for this provider
}
```

Providers wrap API clients and are registered in the `providerapi.ProviderRegistry`. Component managers retrieve providers by name to get their required API clients.

### ProviderRegistry

Manages provider instances. Component manager factories use `providerapi.GetTyped[T]()` to retrieve type-safe providers:

```go
provider, err := providerapi.GetTyped[*nico.Provider](
    providerRegistry,
    nico.ProviderName,
)
```

### ComponentManager Interface

```go
type ComponentManager interface {
    Type() devicetypes.ComponentType
    InjectExpectation(ctx, target, info) error
    PowerControl(ctx, target, info) error
    FirmwareControl(ctx, target, info) error
    GetFirmwareStatus(ctx, target) (map, error)
    GetPowerStatus(ctx, target) (map, error)
}
```

### ManagerFactory

```go
type ManagerFactory func(providers *providerapi.ProviderRegistry) (ComponentManager, error)
```

Factory functions create component manager instances. They receive the `ProviderRegistry` to retrieve any required providers.

### Registry

The `Registry` stores factories and active managers:
- `RegisterFactory()` - Register a factory for a component type + implementation name
- `Initialize()` - Create managers based on configuration
- `GetManager()` - Retrieve active manager for a component type, returning a
  descriptive error when the registry is not configured or no manager is active
- `FindManager()` - Probe for an active manager, returning nil when absent

## Directory Structure

```
internal/task/componentmanager/
├── componentmanager.go      # ComponentManager interface, Registry
├── providerapi/             # Provider interfaces and registries
├── config.go                # Configuration parsing
├── mock/
│   └── mock.go              # Generic mock implementation
├── providers/
│   ├── nico/
│   │   └── provider.go      # NICo API provider
│   └── psm/
│       └── provider.go      # PSM API provider
├── compute/
│   └── nico/
│       └── nico.go       # NICo-based compute manager
├── nvlswitch/
│   └── nico/
│       └── nico.go       # NICo-based NVL switch manager
└── powershelf/
    └── psm/
        └── psm.go           # PSM-based power shelf manager
```

---

## Adding a New Provider

Follow these steps to add a new API provider (e.g., a new external service).

### Step 1: Create the Provider Package

Create `internal/task/componentmanager/providers/<name>/provider.go`:

```go
package myapi

import (
    "time"
    "github.com/rs/zerolog/log"
    "github.com/NVIDIA/infra-controller-rest/rla/internal/myapi"  // Your API client
)

const (
    ProviderName   = "myapi"
    DefaultTimeout = 30 * time.Second
)

// Config holds configuration for the provider.
type Config struct {
    Timeout time.Duration
}

// Provider wraps the API client.
type Provider struct {
    client myapi.Client
}

// New creates a new Provider using the provided configuration.
func New(config Config) (*Provider, error) {
    client, err := myapi.NewClient(config.Timeout)
    if err != nil {
        log.Error().Err(err).Msg("Failed to create MyAPI client")
        return nil, err
    }
    return &Provider{client: client}, nil
}

// NewFromClient creates a Provider from an existing client (for testing).
func NewFromClient(client myapi.Client) *Provider {
    return &Provider{client: client}
}

// Name returns the unique identifier for this provider.
func (p *Provider) Name() string {
    return ProviderName
}

// Client returns the underlying API client.
func (p *Provider) Client() myapi.Client {
    return p.client
}
```

### Step 2: Add Configuration Support

Provider-specific configuration lives with the provider package. Add a
`ConfigDecoder` that implements the `providerapi.ProviderConfigDecoder`
interface:

```go
type ConfigDecoder struct{}

func (ConfigDecoder) Name() string {
    return ProviderName
}

func (ConfigDecoder) DefaultConfig() providerapi.ProviderConfig {
    return &Config{Timeout: DefaultTimeout}
}

func (d ConfigDecoder) DecodeYAML(raw yaml.Node) (providerapi.ProviderConfig, error) {
    config := d.DefaultConfig().(*Config)
    // Decode provider-specific YAML into config.
    return config, nil
}
```

Generic YAML parsing and validation lives in
`internal/task/componentmanager/config`. That package should not import
provider implementations directly.

### Step 3: Register the Provider Decoder

Update the service-supported provider catalog in
`internal/task/componentmanager/builtin`:

```go
func serviceProviderConfigDecoders() []providerapi.ProviderConfigDecoder {
    return []providerapi.ProviderConfigDecoder{
        nico.ConfigDecoder{},
        psm.ConfigDecoder{},
        myapi.ConfigDecoder{},
    }
}
```

`cmd/serve.go` does not need provider-specific construction code. It loads the
service config through `builtin.LoadConfig`, then creates providers from the
decoded generic provider configs. Built-in provider decoders and component
manager factory registration both live in `internal/task/componentmanager/builtin`.

---

## Adding a New Component Manager Implementation

Follow these steps to add a new implementation for an existing component type.

### Step 1: Create the Implementation Package

Create `internal/task/componentmanager/<component_type>/<impl_name>/<impl_name>.go`:

```go
package myimpl

import (
    "context"
    "fmt"

    "github.com/NVIDIA/infra-controller-rest/rla/internal/task/componentmanager"
    "github.com/NVIDIA/infra-controller-rest/rla/internal/task/componentmanager/providerapi"
    myapiprovider "github.com/NVIDIA/infra-controller-rest/rla/internal/task/componentmanager/providers/myapi"
    "github.com/NVIDIA/infra-controller-rest/rla/internal/task/executor/temporalworkflow/common"
    "github.com/NVIDIA/infra-controller-rest/rla/internal/task/operations"
    "github.com/NVIDIA/infra-controller-rest/rla/pkg/common/devicetypes"
)

const ImplementationName = "myimpl"

// Manager implements ComponentManager using MyAPI.
type Manager struct {
    client myapi.Client
}

// New creates a new Manager instance.
func New(client myapi.Client) *Manager {
    return &Manager{client: client}
}

// Factory creates a Manager from the ProviderRegistry.
func Factory(providers *providerapi.ProviderRegistry) (componentmanager.ComponentManager, error) {
    provider, err := providerapi.GetTyped[*myapiprovider.Provider](
        providers,
        myapiprovider.ProviderName,
    )
    if err != nil {
        return nil, fmt.Errorf("myimpl requires myapi provider: %w", err)
    }
    return New(provider.Client()), nil
}

// Register registers this implementation with the registry.
func Register(registry *componentmanager.Registry) {
    registry.RegisterFactory(devicetypes.ComponentTypeCompute, ImplementationName, Factory)
}

// Type returns the component type.
func (m *Manager) Type() devicetypes.ComponentType {
    return devicetypes.ComponentTypeCompute
}

// InjectExpectation implements ComponentManager.
func (m *Manager) InjectExpectation(ctx context.Context, target common.Target, info operations.InjectExpectationTaskInfo) error {
    // Implementation here
}

// PowerControl implements ComponentManager.
func (m *Manager) PowerControl(ctx context.Context, target common.Target, info operations.PowerControlTaskInfo) error {
    // Implementation here
}

// FirmwareControl implements ComponentManager.
func (m *Manager) FirmwareControl(ctx context.Context, target common.Target, info operations.FirmwareControlTaskInfo) error {
    // Implementation here — initiate firmware update, return immediately
}
```

### Step 2: Register the Implementation

Update the service-supported manager catalog in
`internal/task/componentmanager/builtin`:

```go
import (
    myimpl "github.com/NVIDIA/infra-controller-rest/rla/internal/task/componentmanager/compute/myimpl"
)

func serviceComponentManagerRegistrars(config cmconfig.Config) ([]componentManagerRegistrar, error) {
    return []componentManagerRegistrar{
        // ... existing registrations ...
        myimpl.Register, // Add new implementation
    }, nil
}
```

### Step 3: Use in Configuration

Now you can use the new implementation in YAML config:

```yaml
component_managers:
  compute: myimpl
  nvlswitch: nico
  powershelf: psm

providers:
  myapi:
    timeout: "30s"
  nico:
    timeout: "1m"
  psm:
    timeout: "30s"
```

---

## Adding a New Component Type

To add an entirely new component type (e.g., `gpu`):

1. Add the type to `pkg/common/devicetypes/component.go`
2. Create implementation(s) under `internal/task/componentmanager/gpu/<impl>/`
3. Update the mock in `internal/task/componentmanager/mock/mock.go` to include it in `RegisterAll()`
4. Update configuration parsing to recognize the new type

---

## Testing

### Unit Testing with Mock Providers

```go
func TestManager(t *testing.T) {
    mockClient := &MockMyAPIClient{}
    manager := myimpl.New(mockClient)
    
    err := manager.PowerControl(ctx, target, info)
    assert.NoError(t, err)
}
```

### Integration Testing with Mock Implementation

Use the mock implementation in test configuration:

```yaml
component_managers:
  compute: mock
  nvlswitch: mock
  powershelf: mock
```
