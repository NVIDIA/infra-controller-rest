# Tray / Rack Rule Split — Design Document

**Status**: Draft  
**Author**: TBD  
**Last updated**: 2026-05-18

## 1. Background and Problem

### 1.1 Current State

Flow's operation rule model (`flow/internal/task/operationrules/`) today has these properties:

- An `OperationRule` is keyed by `(operation_type, operation_code)`, with optional
  per-rack overrides via the `rack_rule_associations` table.
- Each rule's body is a `[]SequenceStep`. Every step carries both:
  - **Per-tray behavior**: `pre_operation` / `main_operation` / `post_operation` /
    `timeout` / `retry_policy` / `max_parallel`;
  - **Rack-level orchestration**: `stage` (sequence number), `component_type`.
- The resolver entry point `ResolveRule(opType, opCode, rackID, ruleID)` uses the
  same lookup path whether the target is a single tray or a whole rack.

### 1.2 Problems

A single rule conflates two semantic dimensions:

1. **Tray operations are forced through rack-scope rules.**  
   Even when a user powers on one Compute tray, the task manager still looks up
   `rack_rule_associations` and applies a multi-stage rack rule. Other stages are
   skipped in `helpers.go`, but the semantics and implementation are not clean.
2. **Tray behavior cannot be tuned independently.**  
   Changing "Compute tray `power_on` timeout to 30 minutes" requires editing a
   rack-scope rule that also mixes PowerShelf / NVLSwitch stages.
3. **Per-rack overrides leak into tray operations.**  
   After a rack gets a special `power_on` rack rule, single-tray `power_on` on that
   rack follows the same rule — surprising and wrong.
4. **Conceptual coupling.**  
   Tray behavior ("what to do when powering on this Compute tray") and rack
   orchestration ("PowerShelf → NVLSwitch → Compute order") are orthogonal and
   should live in separate layers.

### 1.3 Goals

- **Split the two semantic dimensions**: tray layer defines per-component-type
  behavior; rack layer defines orchestration across trays (order, parallelism,
  nesting).
- **Rack rules compose tray rules** ("tree / nested" model): rack rules are
  compositions; tray rules are atomic primitives.
- **Tray operations resolve tray rules directly**, without rack rule lookup.
- **No proto / gRPC API changes, no carbide-rest caller changes** (backward
  compatible).
- **Conflict detection remains rack-scoped** (unchanged).

## 2. Conceptual Model

```
┌─────────────────────────────────────────────────────────┐
│ RackRule (op_type, op_code)                             │
│                                                         │
│   Stage 1 (parallel within stage)                       │
│     └─ TrayRuleRef { component_type: PowerShelf }       │──┐
│                                                            │
│   Stage 2                                                  │
│     ├─ TrayRuleRef { component_type: NVLSwitch }        │──┤
│     └─ TrayRuleRef { component_type: Compute   }        │──┤  references
│                                                            │  (by id or
│   Stage 3                                                  │   by op key)
│     └─ TrayRuleRef { component_type: Compute   }        │──┤
└────────────────────────────────────────────────────────────┘
                                                              │
   ┌──────────────────────────────────────────────────────────┘
   ▼
┌──────────────────────────────────────────────────────┐
│ TrayRule (op_type, op_code, component_type)          │  ◀── leaf
│                                                      │
│   pre_operation  : []ActionConfig                    │
│   main_operation : ActionConfig                      │
│   post_operation : []ActionConfig                    │
│   timeout        : Duration                          │
│   max_parallel   : int                               │
│   retry_policy   : RetryPolicy                       │
└──────────────────────────────────────────────────────┘
```

**Two node kinds:**

- **TrayRule**: Leaf node. Bound to `(operation_type, operation_code,
  component_type)`. Describes pre / main / post behavior for that tray type and
  operation — the smallest reusable, composable unit.
- **RackRule**: Composition node. Bound to `(operation_type, operation_code)`.
  Consists of stages; each stage holds one or more references to tray rules. Stages
  run sequentially; refs within a stage run in parallel.

**Two resolution paths:**

- **Tray-scope operations** (single component type): resolve `TrayRule` only; do
  not consult `RackRule`.
- **Rack-scope operations** (multiple component types or explicit whole-rack
  target): resolve `RackRule`, expand stages into a sequence of tray rules, then
  execute.

## 3. Schema Design

### 3.1 TrayRule (leaf)

```go
// Go model
type TrayRule struct {
    ID            uuid.UUID
    Name          string
    Description   string
    OperationType common.TaskType            // power_control / firmware_control / bring_up …
    OperationCode string                     // power_on / power_off / upgrade …
    ComponentType devicetypes.ComponentType  // Compute / NVLSwitch / PowerShelf
    Definition    TrayRuleDefinition         // see below
    IsDefault     bool
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

type TrayRuleDefinition struct {
    Version       string         `json:"version"`
    MaxParallel   int            `json:"max_parallel"`
    Timeout       time.Duration  `json:"timeout"`
    RetryPolicy   *RetryPolicy   `json:"retry,omitempty"`
    PreOperation  []ActionConfig `json:"pre_operation,omitempty"`
    MainOperation ActionConfig   `json:"main_operation"`
    PostOperation []ActionConfig `json:"post_operation,omitempty"`
}
```

```sql
-- DDL
CREATE TABLE tray_operation_rules (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            varchar(128) NOT NULL,
    description     text,
    operation_type  varchar(64) NOT NULL,
    operation_code  varchar(64) NOT NULL,
    component_type  varchar(32) NOT NULL,
    definition      jsonb NOT NULL,
    is_default      boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- At most one default per (op_type, op_code, component_type)
CREATE UNIQUE INDEX uq_tray_rules_default
  ON tray_operation_rules(operation_type, operation_code, component_type)
  WHERE is_default = true;

CREATE INDEX idx_tray_rules_lookup
  ON tray_operation_rules(operation_type, operation_code, component_type);
```

### 3.2 RackRule (composition)

```go
type RackRule struct {
    ID            uuid.UUID
    Name          string
    Description   string
    OperationType common.TaskType
    OperationCode string
    Definition    RackRuleDefinition
    IsDefault     bool
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

type RackRuleDefinition struct {
    Version string          `json:"version"`
    Stages  []RackRuleStage `json:"stages"`
}

type RackRuleStage struct {
    Stage int            `json:"stage"`       // sequence number; same number ⇒ parallel
    Refs  []TrayRuleRef  `json:"refs"`        // tray rules in this stage
}

// Reference by (op_type, op_code, component_type) rather than tray_rule_id so tray
// rules can be replaced / upgraded without rewriting rack rules.
// To pin a specific tray rule, set RuleID (optional pointer).
type TrayRuleRef struct {
    ComponentType devicetypes.ComponentType `json:"component_type"`
    RuleID        *uuid.UUID                `json:"rule_id,omitempty"`
}
```

```sql
CREATE TABLE rack_operation_rules (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            varchar(128) NOT NULL,
    description     text,
    operation_type  varchar(64) NOT NULL,
    operation_code  varchar(64) NOT NULL,
    definition      jsonb NOT NULL,
    is_default      boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uq_rack_rules_default
  ON rack_operation_rules(operation_type, operation_code)
  WHERE is_default = true;
```

`rack_rule_associations` keeps the same semantics; `rule_id` now references
`rack_operation_rules`:

```sql
ALTER TABLE rack_rule_associations
  DROP CONSTRAINT rack_rule_associations_rule_fkey,
  ADD  CONSTRAINT rack_rule_associations_rule_fkey
       FOREIGN KEY (rule_id) REFERENCES rack_operation_rules(id) ON DELETE CASCADE;
```

> Tray rules do not get per-rack associations initially. Add `tray_rule_associations`
> later if there is a real need for "rack A's Compute tray behaves differently from
> rack B's." YAGNI.

### 3.3 Full nested tree vs fixed two layers

The original proposal was a nested / tree model. Two implementation options:

| Option | Description | Expressiveness | Complexity |
|---|---|---|---|
| A (two layers) | Fixed RackRule → Stage → TrayRule | Covers 100% of existing rules | Low |
| B (recursive) | A stage node can be a TrayRule or another RackRule (subtree) | Nested sub-sequences | Medium–high |

**Recommend A first.** Rationale:

1. All current hardcoded rules (power_on / power_off / restart / force_* /
   bring_up / firmware) are flat stage lists; A is sufficient.
2. B adds cycle detection, depth limits, and definition-time validation.
3. Upgrading A → B is a superset; schema stays compatible (add optional
   `child_rack_rule_id` on stages; old data unchanged).

If a real need appears for "run sub-sequence X, then sub-sequence Y" where X and Y
are each multi-stage, bump `RackRuleDefinition` in a minor version to allow
nesting.

## 4. Resolver Changes

### 4.1 Scope detection

Tray vs rack scope is decided in `manager.createAndExecuteTask`, **after**
`compsByType` is built and **before** calling the resolver:

```go
// Pseudocode — flow/internal/task/manager/manager.go
isTrayScope := isTrayScopeOperation(req, compsByType)

if isTrayScope {
    var ct devicetypes.ComponentType
    for k := range compsByType { ct = k }   // exactly one key
    trayRule, err := m.ruleResolver.ResolveTrayRule(
        ctx, task.Operation.Type, task.Operation.Code, ct, ruleID,
    )
    // … build a single-stage execution plan from trayRule
} else {
    rackRule, err := m.ruleResolver.ResolveRackRule(
        ctx, task.Operation.Type, task.Operation.Code, task.RackID, ruleID,
    )
    // … expand rack rule
}

func isTrayScopeOperation(
    req *operation.Request,
    compsByType map[devicetypes.ComponentType][]uuid.UUID,
) bool {
    // Explicit whole-rack (RackTargets with no component_types filter) ⇒ rack scope
    if isExplicitWholeRack(&req.TargetSpec) {
        return false
    }
    // Otherwise: single component type ⇒ tray scope
    return len(compsByType) == 1
}

func isExplicitWholeRack(ts *operation.TargetSpec) bool {
    if !ts.IsRackTargeting() {
        return false
    }
    for _, rt := range ts.Racks {
        if len(rt.ComponentTypes) == 0 {
            return true
        }
    }
    return false
}
```

Decision matrix:

| Client intent | Proto shape | `compsByType` | Scope |
|---|---|---|---|
| Power on whole rack | `RackTargets{no filter}` | multiple types | rack |
| Power on rack, Compute only | `RackTargets{types=[Compute]}` | {Compute} | **tray** |
| Power on one Compute tray | `ComponentTargets{[uuid]}` | {Compute} | **tray** |
| Power on five Compute trays | `ComponentTargets{[5×uuid]}` | {Compute} | **tray** |
| Whole rack but only Compute remains | `RackTargets{no filter}` | {Compute} | rack (explicit wins) |
| Whole site (`tray.go` TrayFilter nil) | `RackTargets{types=ALL}` | multiple types | rack |

### 4.2 TrayRule resolution order

```
ResolveTrayRule(opType, opCode, componentType, explicitRuleID):
  1. If explicitRuleID != nil → load that tray rule (error if missing / wrong type)
  2. tray_operation_rules WHERE (op_type, op_code, component_type) AND is_default=true
  3. Hardcoded default for (opType, opCode, componentType)
```

### 4.3 RackRule resolution order

```
ResolveRackRule(opType, opCode, rackID, explicitRuleID):
  1. If explicitRuleID != nil → load that rack rule
  2. rack_rule_associations WHERE (rack_id, op_type, op_code) → rack_operation_rules
  3. rack_operation_rules WHERE (op_type, op_code) AND is_default=true
  4. Hardcoded default for (opType, opCode)

Expansion: for each TrayRuleRef in each stage:
  - If ref.RuleID != nil → load that tray rule
  - Else → ResolveTrayRule(opType, opCode, ref.ComponentType, nil)
    (do not pass rackID; tray rules have no per-rack override)
```

### 4.4 Execution plan shape

Executors in `temporalworkflow/workflow/*.go` today consume `[]SequenceStep`. To keep
executors **unaware of tray vs rack**, the resolver flattens both rule kinds into
one plan type:

```go
type ExecutionPlan struct {
    Stages []ExecutionStage
}

type ExecutionStage struct {
    Steps []ExecutionStep   // run in parallel
}

type ExecutionStep struct {
    ComponentType devicetypes.ComponentType
    TrayRule      TrayRuleDefinition  // resolved tray rule inlined
}
```

- Tray scope: one stage, one step.
- Rack scope: stages from the rack rule; each step carries the resolved tray rule.

Executors stay dumb; all resolution complexity lives in the resolver.

## 5. Hardcoded Default Migration

Hardcoded rules in `resolver_defaults.go` (~1300 lines) split as follows.

### 5.1 One PowerOnRule → three TrayRules + one RackRule

**Before** (illustrative):

```yaml
PowerOnRackRule:
  stages:
    - stage: 1, component: PowerShelf, pre: [], main: PowerControl, post: [VerifyPowerStatus on], timeout: 15m, retry: ...
    - stage: 2, component: NVLSwitch,  ...
    - stage: 3, component: Compute,    ...
```

**After**:

```yaml
# Three TrayRules (leaves)
PowerOn_PowerShelf_TrayRule:
  pre: []
  main: PowerControl
  post: [VerifyPowerStatus on]
  timeout: 15m
  retry: { ... }

PowerOn_NVLSwitch_TrayRule: { ... }
PowerOn_Compute_TrayRule:   { ... }

# One RackRule (composition)
PowerOn_RackRule:
  stages:
    - stage: 1, refs: [{ component_type: PowerShelf }]
    - stage: 2, refs: [{ component_type: NVLSwitch }]
    - stage: 3, refs: [{ component_type: Compute   }]
```

### 5.2 Reuse benefits

Grouped by component type, the ~8 hardcoded rules (power_on, force_power_on,
power_off, force_power_off, restart, force_restart, upgrade, bring_up) repeat many
steps per type. After the split, tray rule count ≈ `|ops| × |componentTypes|` ≈
8 × 3 = 24, with substantial sharing (e.g. Compute behavior for the "power on
phase" of power_on and restart can be identical).

### 5.3 Split conventions

- **TrayRule names**: `{op}_{componentType}`, e.g. `power_on_compute`,
  `force_power_off_powershelf`.
- **`main_operation.parameters.operation` in composite rack ops** (restart, etc.):
  tray rules must set `power_on` / `power_off` explicitly in main, not rely on task
  context inheritance — same as today's `buildRestartRule` comment.
- **Bring-up**: bring-up reuses the same component type across stages (e.g. stage 2
  and stage 6 both Compute). After the split, those stages reference different tray
  rules ("Compute power_on" vs "Compute restart"). Today that is two
  `SequenceStep` rows distinguished by stage; the new model allows the same
  `component_type` in different stages to reference **different (op_type, op_code)**
  via optional overrides on `TrayRuleRef`:

```go
type TrayRuleRef struct {
    ComponentType devicetypes.ComponentType
    // Default: use the parent RackRule's (op_type, op_code); override when set.
    OverrideOperationType *common.TaskType
    OverrideOperationCode *string
    RuleID                *uuid.UUID    // absolute pin (highest priority)
}
```

Bring-up stage 6 can be
`{ component: Compute, override_op: power_control/restart }`, reusing the existing
`restart_compute` tray rule.

## 6. Conflict / Concurrency

**Conflict detection stays rack-scoped; logic unchanged:**

- `conflict/conflict.go` `builtinRule` needs no change.
- Tasks are still grouped by rack (`resolveTargetSpecToRacks` unchanged);
  `compsByType` still goes into `task.Attributes`; overlap checks still use
  `ComponentsByType` within a rack.

Related change: tray-scope tasks still occupy the rack's in-flight slot (two tray
ops on the same rack still overlap per existing rules). That is correct — the rack
is the physical resource boundary.

## 7. API and Compatibility

### 7.1 Proto / gRPC

**Unchanged.** `OperationTargetSpec`, `PowerOnRackRequest`, `UpgradeFirmwareRequest`,
and all existing messages stay as-is. Tray vs rack is inferred entirely inside Flow.

The `rule_id` field (e.g. on `PowerOnRackRequest`) gains slightly broader meaning:

- Before: must point at a row in `operation_rules`.
- After: may point at `rack_operation_rules` or `tray_operation_rules`. Flow checks
  that the rule type matches tray vs rack scope for the task; mismatch ⇒
  `InvalidArgument`.

### 7.2 REST API (carbide-rest)

**Unchanged.** `ToTargetSpec` in `api/pkg/api/handler/tray.go` and `rack.go` stays;
inference is on the Flow side.

### 7.3 CLI (nicocli)

New commands:

- `nicocli rule tray list / get / create / update / delete`
- `nicocli rule tray set-default`
- Existing `nicocli rule …` (today's `operation_rules`) becomes `nicocli rule rack …`

Keep old commands as aliases for a transition period with a deprecation warning.

### 7.4 Data migration

- Add `tray_operation_rules` and `rack_operation_rules` (migration up).
- Migrate existing `operation_rules` data:
  - One-off Go migration (not raw SQL): split each legacy rule into `N` tray rules +
    one rack rule. Naming:
    - tray rule name = `{old_rule.name}__{component_type}`
    - rack rule keeps `{old_rule.name}`
  - `rack_rule_associations.rule_id` → new rack rule ids.
  - Keep `operation_rules` read-only for 90 days (feature flag) for rollback; drop
    via `down.sql` afterward.
- Deprecate `operationrules.OperationRule` in Go; new code uses `TrayRule` /
  `RackRule`.

## 8. Implementation Plan (recommended order)

1. **Types + tables**: `TrayRule` / `RackRule` Go types; new migration (up + down).
   Dual-write phase.
2. **New hardcoded defaults**: split the eight hardcoded rack rules into tray +
   rack defaults as fallback. Keep migration notes in comments.
3. **New resolver**: `ResolveTrayRule` / `ResolveRackRule`; `ExecutionPlan` type.
   Keep old `ResolveRule` as a shim forwarding to the new APIs.
4. **Manager scope detection**: `isTrayScopeOperation` branch in
   `createAndExecuteTask`.
5. **Executor flatten**: executors consume `ExecutionPlan` instead of
   `RuleDefinition`.
6. **DAO + store**: CRUD for `tray_operation_rules`; adapt rack rule CRUD.
7. **gRPC**: split rule-admin RPCs (e.g. `CreateOperationRule`) into
   `CreateTrayOperationRule` / `CreateRackOperationRule`. Task execution RPCs
   unchanged.
8. **Data migration program**: migrate `operation_rules` rows.
9. **CLI**: add `rule tray …`; deprecate old `rule …`.
10. **Docs**: update `operation-rules-guide.md`, `grpc-api.md`,
    `operation-rules-versioning.md`.
11. **Remove legacy path**: after 90 days, drop `operation_rules` and old resolver.

Each step can be its own PR. Production behavior stays stable through steps 1–10
(dual-write, resolver shim).

## 9. Alternatives

### 9.1 Option X: Resolver-only projection

When a task touches only one component type, `ResolveRule` projects the rack rule
to a single `SequenceStep` and skips `rack_rule_associations`.

- ✅ Smallest change (resolver only)
- ❌ Schema still mixed; tuning tray behavior still edits rack rules
- ❌ Short-circuits association lookup, not real layering
- Fit: emergency fix, short-lived PR

### 9.2 Option Y: Proto `OperationScope` enum

Clients declare tray vs rack scope explicitly.

- ✅ Clear at the API boundary
- ❌ Proto and SDK changes
- ❌ Duplicates `target_spec`; client mistakes cause subtler bugs
- ❌ Tray-by-UUID still needs DAO lookup for component type

### 9.3 This document (Option Z): Split schema + internal inference

- ✅ Proto / API unchanged
- ✅ Clear model, high tray-rule reuse
- ✅ Smooth path to nested tree later
- ❌ Larger effort (schema migration + CLI + docs)

**Conclusion**: Option X for emergencies; Option Z for the long term. Option Y is
not recommended.

## 10. Open Questions

1. **Per-rack overrides for TrayRule?**  
   Rack rules keep `rack_rule_associations`; tray rules do not initially. Revisit if
   "rack A's Compute retry policy differs from other racks" becomes a requirement.
2. **Abuse of `TrayRuleRef.OverrideOperationCode` on long bring-up rack rules?**  
   Validate at rule creation: overridden `(op_type, op_code, component_type)` must
   resolve to an existing tray rule.
3. **Is `is_default` still needed on rack rules?**  
   Yes. Whole-rack operations still need a fallback not tied to a specific rack —
   same semantics as today.
4. **`rule_definition` JSON schema versioning**  
   Tray and rack rules each carry their own `version` field and evolve independently.
   Update `operation-rules-versioning.md` accordingly.
5. **CLI "show full execution plan"?**  
   Add e.g. `nicocli rule rack explain --rack-id <id> --op power_on` to print the
   resolved `ExecutionPlan` (tray rules inlined) for human review.

## 11. Non-Goals

- Rewriting conflict detection.
- Changing Temporal workflow / activity interfaces (at most an `ExecutionPlan`
  adapter at the executor entry).
- Changing carbide-rest / nicocli interaction patterns (only new subcommands).
- Fully recursive rule trees up front (Option A first).
