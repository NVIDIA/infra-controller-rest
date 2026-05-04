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
│  │   carbide   │  │     psm     │  │   (new...)  │                 │
│  │  Provider   │  │  Provider   │  │  Provider   │                 │
│  └─────────────┘  └─────────────┘  └─────────────┘                 │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    ComponentManager Registry                        │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ ComponentType: Compute                                       │   │
│  │   ├── "carbide" → Factory → Manager (uses carbide.Provider)  │   │
│  │   └── "mock"    → Factory → Manager (no provider needed)     │   │
│  ├─────────────────────────────────────────────────────────────┤   │
│  │ ComponentType: NVLSwitch                                     │   │
│  │   ├── "carbide" → Factory → Manager                          │   │
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

Providers wrap API clients and are registered in the `ProviderRegistry`. Component managers retrieve providers by name to get their required API clients.

### ProviderRegistry

Manages provider instances. Component manager factories use `GetTyped[T]()` to retrieve type-safe providers:

```go
provider, err := componentmanager.GetTyped[*carbide.Provider](
    providerRegistry,
    carbide.ProviderName,
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
type ManagerFactory func(providers *ProviderRegistry) (ComponentManager, error)
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
├── provider.go              # Provider interface, ProviderRegistry
├── config.go                # Configuration parsing
├── mock/
│   └── mock.go              # Generic mock implementation
├── providers/
│   ├── carbide/
│   │   └── provider.go      # Carbide API provider
│   └── psm/
│       └── provider.go      # PSM API provider
├── compute/
│   └── carbide/
│       └── carbide.go       # Carbide-based compute manager
├── nvlswitch/
│   └── carbide/
│       └── carbide.go       # Carbide-based NVL switch manager
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
    "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/myapi"  // Your API client
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

Update `internal/task/componentmanager/config.go`:

```go
import (
    // ... existing imports
    "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/myapi"
)

type ProviderConfig struct {
    Carbide *carbide.Config
    PSM     *psm.Config
    MyAPI   *myapi.Config  // Add new provider config
}

type rawProviderConfig struct {
    Carbide *rawCarbideConfig `yaml:"carbide"`
    PSM     *rawPSMConfig     `yaml:"psm"`
    MyAPI   *rawMyAPIConfig   `yaml:"myapi"`  // Add raw config
}

type rawMyAPIConfig struct {
    Timeout string `yaml:"timeout"`
}
```

Update `ParseConfig()` and `deriveProviders()` to handle the new provider.

### Step 3: Register the Provider

Update `cmd/serve.go` in `initProviderRegistry()`:

```go
import (
    "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/myapi"
)

func initProviderRegistry(config componentmanager.Config) (...) {
    // ... existing providers ...

    // Initialize MyAPI provider if configured
    if config.Providers.MyAPI != nil {
        myapiProvider, err := myapi.New(*config.Providers.MyAPI)
        if err != nil {
            log.Warn().Err(err).Msg("Unable to create MyAPI client")
        } else {
            providerRegistry.Register(myapiProvider)
        }
    }
}
```

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

    "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager"
    myapiprovider "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/myapi"
    "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/executor/temporalworkflow/common"
    "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/operations"
    "github.com/NVIDIA/ncx-infra-controller-rest/rla/pkg/common/devicetypes"
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
func Factory(providers *componentmanager.ProviderRegistry) (componentmanager.ComponentManager, error) {
    provider, err := componentmanager.GetTyped[*myapiprovider.Provider](
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

Update `cmd/serve.go` in `initComponentManagerRegistry()`:

```go
import (
    myimpl "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/compute/myimpl"
)

func initComponentManagerRegistry(...) (*componentmanager.Registry, error) {
    registry := componentmanager.NewRegistry()

    // Register all available component manager factories
    computecarbide.Register(registry)
    myimpl.Register(registry)  // Add new implementation
    // ... other registrations ...
    mock.RegisterAll(registry)

    // ...
}
```

### Step 3: Use in Configuration

Now you can use the new implementation in YAML config:

```yaml
component_managers:
  compute: myimpl
  nvlswitch: carbide
  powershelf: psm

providers:
  myapi:
    timeout: "30s"
  carbide:
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
