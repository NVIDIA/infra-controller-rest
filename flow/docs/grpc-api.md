# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [rla.proto](#rla-proto)
    - [ActualComponent](#v1-ActualComponent)
    - [AttachRacksToNVLDomainRequest](#v1-AttachRacksToNVLDomainRequest)
    - [BMCInfo](#v1-BMCInfo)
    - [BuildInfo](#v1-BuildInfo)
    - [Component](#v1-Component)
    - [ComponentDiff](#v1-ComponentDiff)
    - [ComponentTarget](#v1-ComponentTarget)
    - [ComponentTargets](#v1-ComponentTargets)
    - [CreateExpectedRackRequest](#v1-CreateExpectedRackRequest)
    - [CreateExpectedRackResponse](#v1-CreateExpectedRackResponse)
    - [CreateNVLDomainRequest](#v1-CreateNVLDomainRequest)
    - [CreateNVLDomainResponse](#v1-CreateNVLDomainResponse)
    - [DetachRacksFromNVLDomainRequest](#v1-DetachRacksFromNVLDomainRequest)
    - [DeviceInfo](#v1-DeviceInfo)
    - [DeviceSerialInfo](#v1-DeviceSerialInfo)
    - [ExternalRef](#v1-ExternalRef)
    - [FieldDiff](#v1-FieldDiff)
    - [GetActualComponentsRequest](#v1-GetActualComponentsRequest)
    - [GetActualComponentsResponse](#v1-GetActualComponentsResponse)
    - [GetComponentInfoByIDRequest](#v1-GetComponentInfoByIDRequest)
    - [GetComponentInfoBySerialRequest](#v1-GetComponentInfoBySerialRequest)
    - [GetComponentInfoResponse](#v1-GetComponentInfoResponse)
    - [GetExpectedComponentsRequest](#v1-GetExpectedComponentsRequest)
    - [GetExpectedComponentsResponse](#v1-GetExpectedComponentsResponse)
    - [GetListOfNVLDomainsRequest](#v1-GetListOfNVLDomainsRequest)
    - [GetListOfNVLDomainsResponse](#v1-GetListOfNVLDomainsResponse)
    - [GetListOfRacksRequest](#v1-GetListOfRacksRequest)
    - [GetListOfRacksResponse](#v1-GetListOfRacksResponse)
    - [GetRackInfoByIDRequest](#v1-GetRackInfoByIDRequest)
    - [GetRackInfoBySerialRequest](#v1-GetRackInfoBySerialRequest)
    - [GetRackInfoResponse](#v1-GetRackInfoResponse)
    - [GetRacksForNVLDomainRequest](#v1-GetRacksForNVLDomainRequest)
    - [GetRacksForNVLDomainResponse](#v1-GetRacksForNVLDomainResponse)
    - [GetTasksByIDsRequest](#v1-GetTasksByIDsRequest)
    - [GetTasksByIDsResponse](#v1-GetTasksByIDsResponse)
    - [Identifier](#v1-Identifier)
    - [ListTasksRequest](#v1-ListTasksRequest)
    - [ListTasksResponse](#v1-ListTasksResponse)
    - [Location](#v1-Location)
    - [NVLDomain](#v1-NVLDomain)
    - [OperationTargetSpec](#v1-OperationTargetSpec)
    - [Pagination](#v1-Pagination)
    - [PatchRackRequest](#v1-PatchRackRequest)
    - [PatchRackResponse](#v1-PatchRackResponse)
    - [PowerOffRackRequest](#v1-PowerOffRackRequest)
    - [PowerOnRackRequest](#v1-PowerOnRackRequest)
    - [PowerResetRackRequest](#v1-PowerResetRackRequest)
    - [Rack](#v1-Rack)
    - [RackPosition](#v1-RackPosition)
    - [RackTarget](#v1-RackTarget)
    - [RackTargets](#v1-RackTargets)
    - [StringQueryInfo](#v1-StringQueryInfo)
    - [SubmitTaskResponse](#v1-SubmitTaskResponse)
    - [Task](#v1-Task)
    - [UUID](#v1-UUID)
    - [UpgradeFirmwareRequest](#v1-UpgradeFirmwareRequest)
    - [ValidateComponentsRequest](#v1-ValidateComponentsRequest)
    - [ValidateComponentsResponse](#v1-ValidateComponentsResponse)
    - [VersionRequest](#v1-VersionRequest)
  
    - [BMCType](#v1-BMCType)
    - [ComponentType](#v1-ComponentType)
    - [DiffType](#v1-DiffType)
    - [PowerControlOp](#v1-PowerControlOp)
    - [TaskExecutorType](#v1-TaskExecutorType)
    - [TaskStatus](#v1-TaskStatus)
  
    - [RLA](#v1-RLA)
  
- [Scalar Value Types](#scalar-value-types)



<a name="rla-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## rla.proto



<a name="v1-ActualComponent"></a>

### ActualComponent
ActualComponent represents a component&#39;s actual state from external systems (e.g., NICo, PSM)


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [ComponentType](#v1-ComponentType) |  |  |
| info | [DeviceInfo](#v1-DeviceInfo) |  |  |
| firmware_version | [string](#string) |  |  |
| position | [RackPosition](#v1-RackPosition) |  |  |
| bmcs | [BMCInfo](#v1-BMCInfo) | repeated |  |
| component_id | [string](#string) |  | Component&#39;s own ID from its source system |
| rack_id | [UUID](#v1-UUID) |  | Looked up from local DB using component_id |
| last_seen | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  | Actual-specific fields

When this component was last observed |
| power_state | [string](#string) |  | Current power state |
| health_status | [string](#string) |  | Health status from source system |
| source | [string](#string) |  | Data source (e.g., &#34;nico&#34;, &#34;psm&#34;) |






<a name="v1-AttachRacksToNVLDomainRequest"></a>

### AttachRacksToNVLDomainRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| nvl_domain_identifier | [Identifier](#v1-Identifier) |  |  |
| rack_identifiers | [Identifier](#v1-Identifier) | repeated |  |






<a name="v1-BMCInfo"></a>

### BMCInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [BMCType](#v1-BMCType) |  |  |
| mac_address | [string](#string) |  |  |
| ip_address | [string](#string) | optional |  |
| user | [string](#string) | optional |  |
| password | [string](#string) | optional |  |






<a name="v1-BuildInfo"></a>

### BuildInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| version | [string](#string) |  | e.g., v2025.11.19 |
| build_time | [string](#string) |  | e.g., 2025-01-27T10:30:00Z |
| git_commit | [string](#string) |  | e.g., abc1234 |






<a name="v1-Component"></a>

### Component



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [ComponentType](#v1-ComponentType) |  |  |
| info | [DeviceInfo](#v1-DeviceInfo) |  |  |
| firmware_version | [string](#string) |  |  |
| position | [RackPosition](#v1-RackPosition) |  |  |
| bmcs | [BMCInfo](#v1-BMCInfo) | repeated |  |
| component_id | [string](#string) |  | Component&#39;s own ID from its source system (e.g., NICo machine_id for Compute) |
| rack_id | [UUID](#v1-UUID) |  |  |






<a name="v1-ComponentDiff"></a>

### ComponentDiff



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [DiffType](#v1-DiffType) |  |  |
| component_id | [string](#string) |  |  |
| expected | [Component](#v1-Component) |  | Populated when type is ONLY_IN_EXPECTED |
| actual | [ActualComponent](#v1-ActualComponent) |  | Populated when type is ONLY_IN_ACTUAL |
| field_diffs | [FieldDiff](#v1-FieldDiff) | repeated | Populated when type is DRIFT - lists the fields that differ |






<a name="v1-ComponentTarget"></a>

### ComponentTarget
ComponentTarget identifies a specific component


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| uuid | [string](#string) |  | RLA internal UUID |
| external | [ExternalRef](#v1-ExternalRef) |  | External system reference |






<a name="v1-ComponentTargets"></a>

### ComponentTargets
ComponentTargets contains one or more component targets


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targets | [ComponentTarget](#v1-ComponentTarget) | repeated |  |






<a name="v1-CreateExpectedRackRequest"></a>

### CreateExpectedRackRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rack | [Rack](#v1-Rack) |  |  |






<a name="v1-CreateExpectedRackResponse"></a>

### CreateExpectedRackResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [UUID](#v1-UUID) |  |  |






<a name="v1-CreateNVLDomainRequest"></a>

### CreateNVLDomainRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| nvl_domain | [NVLDomain](#v1-NVLDomain) |  |  |






<a name="v1-CreateNVLDomainResponse"></a>

### CreateNVLDomainResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [UUID](#v1-UUID) |  |  |






<a name="v1-DetachRacksFromNVLDomainRequest"></a>

### DetachRacksFromNVLDomainRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rack_identifiers | [Identifier](#v1-Identifier) | repeated |  |






<a name="v1-DeviceInfo"></a>

### DeviceInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [UUID](#v1-UUID) |  |  |
| name | [string](#string) |  |  |
| manufacturer | [string](#string) |  |  |
| model | [string](#string) | optional |  |
| serial_number | [string](#string) |  |  |
| description | [string](#string) | optional |  |






<a name="v1-DeviceSerialInfo"></a>

### DeviceSerialInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| manufacturer | [string](#string) |  |  |
| serial_number | [string](#string) |  |  |






<a name="v1-ExternalRef"></a>

### ExternalRef
ExternalRef identifies a component by its external system ID.
The component type determines which external system to query
(e.g., COMPUTE -&gt; NICo, POWERSHELF -&gt; PSM)


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [ComponentType](#v1-ComponentType) |  | Component type determines the source system |
| id | [string](#string) |  | ID in that system (e.g., NICo machine_id, PSM PMC MAC) |






<a name="v1-FieldDiff"></a>

### FieldDiff



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| field_name | [string](#string) |  | e.g., &#34;position.slot_id&#34;, &#34;firmware_version&#34; |
| expected_value | [string](#string) |  |  |
| actual_value | [string](#string) |  |  |






<a name="v1-GetActualComponentsRequest"></a>

### GetActualComponentsRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| target_spec | [OperationTargetSpec](#v1-OperationTargetSpec) |  | Flexible targeting: rack(s) with optional type filter, or specific components |
| pagination | [Pagination](#v1-Pagination) | optional |  |






<a name="v1-GetActualComponentsResponse"></a>

### GetActualComponentsResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| components | [ActualComponent](#v1-ActualComponent) | repeated |  |
| total | [int32](#int32) |  |  |






<a name="v1-GetComponentInfoByIDRequest"></a>

### GetComponentInfoByIDRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [UUID](#v1-UUID) |  |  |
| with_rack | [bool](#bool) |  |  |






<a name="v1-GetComponentInfoBySerialRequest"></a>

### GetComponentInfoBySerialRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| serial_info | [DeviceSerialInfo](#v1-DeviceSerialInfo) |  |  |
| with_rack | [bool](#bool) |  |  |






<a name="v1-GetComponentInfoResponse"></a>

### GetComponentInfoResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| component | [Component](#v1-Component) |  |  |
| rack | [Rack](#v1-Rack) |  |  |






<a name="v1-GetExpectedComponentsRequest"></a>

### GetExpectedComponentsRequest
GetExpectedComponents - retrieves expected components from local database


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| target_spec | [OperationTargetSpec](#v1-OperationTargetSpec) |  | Flexible targeting: rack(s) with optional type filter, or specific components |
| pagination | [Pagination](#v1-Pagination) | optional |  |






<a name="v1-GetExpectedComponentsResponse"></a>

### GetExpectedComponentsResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| components | [Component](#v1-Component) | repeated |  |
| total | [int32](#int32) |  |  |






<a name="v1-GetListOfNVLDomainsRequest"></a>

### GetListOfNVLDomainsRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| info | [StringQueryInfo](#v1-StringQueryInfo) |  |  |
| pagination | [Pagination](#v1-Pagination) | optional |  |






<a name="v1-GetListOfNVLDomainsResponse"></a>

### GetListOfNVLDomainsResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| nvl_domains | [NVLDomain](#v1-NVLDomain) | repeated |  |
| total | [int32](#int32) |  |  |






<a name="v1-GetListOfRacksRequest"></a>

### GetListOfRacksRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| info | [StringQueryInfo](#v1-StringQueryInfo) |  |  |
| with_components | [bool](#bool) |  |  |
| pagination | [Pagination](#v1-Pagination) | optional |  |






<a name="v1-GetListOfRacksResponse"></a>

### GetListOfRacksResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| racks | [Rack](#v1-Rack) | repeated |  |
| total | [int32](#int32) |  |  |






<a name="v1-GetRackInfoByIDRequest"></a>

### GetRackInfoByIDRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [UUID](#v1-UUID) |  |  |
| with_components | [bool](#bool) |  |  |






<a name="v1-GetRackInfoBySerialRequest"></a>

### GetRackInfoBySerialRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| serial_info | [DeviceSerialInfo](#v1-DeviceSerialInfo) |  |  |
| with_components | [bool](#bool) |  |  |






<a name="v1-GetRackInfoResponse"></a>

### GetRackInfoResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rack | [Rack](#v1-Rack) |  |  |






<a name="v1-GetRacksForNVLDomainRequest"></a>

### GetRacksForNVLDomainRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| nvl_domain_identifier | [Identifier](#v1-Identifier) |  |  |






<a name="v1-GetRacksForNVLDomainResponse"></a>

### GetRacksForNVLDomainResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| racks | [Rack](#v1-Rack) | repeated |  |






<a name="v1-GetTasksByIDsRequest"></a>

### GetTasksByIDsRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| task_ids | [UUID](#v1-UUID) | repeated |  |






<a name="v1-GetTasksByIDsResponse"></a>

### GetTasksByIDsResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tasks | [Task](#v1-Task) | repeated |  |






<a name="v1-Identifier"></a>

### Identifier



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [UUID](#v1-UUID) |  |  |
| name | [string](#string) |  |  |






<a name="v1-ListTasksRequest"></a>

### ListTasksRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rack_id | [UUID](#v1-UUID) | optional |  |
| active_only | [bool](#bool) |  |  |
| pagination | [Pagination](#v1-Pagination) | optional |  |






<a name="v1-ListTasksResponse"></a>

### ListTasksResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tasks | [Task](#v1-Task) | repeated |  |
| total | [int32](#int32) |  |  |






<a name="v1-Location"></a>

### Location



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| region | [string](#string) |  |  |
| datacenter | [string](#string) |  |  |
| room | [string](#string) |  |  |
| position | [string](#string) |  |  |






<a name="v1-NVLDomain"></a>

### NVLDomain



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| identifier | [Identifier](#v1-Identifier) |  |  |






<a name="v1-OperationTargetSpec"></a>

### OperationTargetSpec
OperationTargetSpec contains targets for an operation.
Supports either rack-level targeting (with optional type filtering)
or component-level targeting (by UUID or external reference), but not both.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| racks | [RackTargets](#v1-RackTargets) |  |  |
| components | [ComponentTargets](#v1-ComponentTargets) |  |  |






<a name="v1-Pagination"></a>

### Pagination



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| offset | [int32](#int32) |  |  |
| limit | [int32](#int32) |  |  |






<a name="v1-PatchRackRequest"></a>

### PatchRackRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rack | [Rack](#v1-Rack) |  |  |






<a name="v1-PatchRackResponse"></a>

### PatchRackResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| report | [string](#string) |  |  |






<a name="v1-PowerOffRackRequest"></a>

### PowerOffRackRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| target_spec | [OperationTargetSpec](#v1-OperationTargetSpec) |  | Flexible targeting: rack(s) with optional type filter, or specific components |
| forced | [bool](#bool) |  |  |
| description | [string](#string) |  | optional task description |






<a name="v1-PowerOnRackRequest"></a>

### PowerOnRackRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| target_spec | [OperationTargetSpec](#v1-OperationTargetSpec) |  | Flexible targeting: rack(s) with optional type filter, or specific components |
| description | [string](#string) |  | optional task description |






<a name="v1-PowerResetRackRequest"></a>

### PowerResetRackRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| target_spec | [OperationTargetSpec](#v1-OperationTargetSpec) |  | Flexible targeting: rack(s) with optional type filter, or specific components |
| forced | [bool](#bool) |  |  |
| description | [string](#string) |  | optional task description |






<a name="v1-Rack"></a>

### Rack



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| info | [DeviceInfo](#v1-DeviceInfo) |  |  |
| location | [Location](#v1-Location) |  |  |
| components | [Component](#v1-Component) | repeated |  |






<a name="v1-RackPosition"></a>

### RackPosition



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| slot_id | [int32](#int32) |  |  |
| tray_idx | [int32](#int32) |  |  |
| host_id | [int32](#int32) |  |  |






<a name="v1-RackTarget"></a>

### RackTarget
RackTarget identifies a rack and optionally filters by component types


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | Rack UUID |
| name | [string](#string) |  | Rack name |
| component_types | [ComponentType](#v1-ComponentType) | repeated | Optional: component types to include. Empty = ALL components in rack |






<a name="v1-RackTargets"></a>

### RackTargets
RackTargets contains one or more rack targets


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targets | [RackTarget](#v1-RackTarget) | repeated |  |






<a name="v1-StringQueryInfo"></a>

### StringQueryInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| patterns | [string](#string) | repeated |  |
| is_wildcard | [bool](#bool) |  |  |
| use_or | [bool](#bool) |  |  |






<a name="v1-SubmitTaskResponse"></a>

### SubmitTaskResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| task_ids | [UUID](#v1-UUID) | repeated | Multiple task IDs (1 task per rack) |






<a name="v1-Task"></a>

### Task



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [UUID](#v1-UUID) |  |  |
| operation | [string](#string) |  |  |
| rack_id | [UUID](#v1-UUID) |  |  |
| component_uuids | [UUID](#v1-UUID) | repeated |  |
| description | [string](#string) |  |  |
| executor_type | [TaskExecutorType](#v1-TaskExecutorType) |  |  |
| execution_id | [string](#string) |  |  |
| status | [TaskStatus](#v1-TaskStatus) |  |  |
| message | [string](#string) |  |  |






<a name="v1-UUID"></a>

### UUID



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="v1-UpgradeFirmwareRequest"></a>

### UpgradeFirmwareRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| target_spec | [OperationTargetSpec](#v1-OperationTargetSpec) |  | required: identifies components to upgrade |
| target_version | [string](#string) | optional | optional: target firmware version |
| start_time | [google.protobuf.Timestamp](#google-protobuf-Timestamp) | optional | optional: scheduled start time |
| end_time | [google.protobuf.Timestamp](#google-protobuf-Timestamp) | optional | optional: scheduled end time |
| description | [string](#string) |  | optional: task description |






<a name="v1-ValidateComponentsRequest"></a>

### ValidateComponentsRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| target_spec | [OperationTargetSpec](#v1-OperationTargetSpec) |  | Flexible targeting: rack(s) with optional type filter, or specific components |






<a name="v1-ValidateComponentsResponse"></a>

### ValidateComponentsResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| diffs | [ComponentDiff](#v1-ComponentDiff) | repeated |  |
| total_diffs | [int32](#int32) |  |  |
| only_in_expected_count | [int32](#int32) |  | Summary counts |
| only_in_actual_count | [int32](#int32) |  |  |
| drift_count | [int32](#int32) |  |  |
| match_count | [int32](#int32) |  |  |






<a name="v1-VersionRequest"></a>

### VersionRequest
Version API messages





 


<a name="v1-BMCType"></a>

### BMCType


| Name | Number | Description |
| ---- | ------ | ----------- |
| BMC_TYPE_UNKNOWN | 0 |  |
| BMC_TYPE_HOST | 1 |  |
| BMC_TYPE_DPU | 2 |  |



<a name="v1-ComponentType"></a>

### ComponentType


| Name | Number | Description |
| ---- | ------ | ----------- |
| COMPONENT_TYPE_UNKNOWN | 0 |  |
| COMPONENT_TYPE_COMPUTE | 1 |  |
| COMPONENT_TYPE_NVLSWITCH | 2 |  |
| COMPONENT_TYPE_POWERSHELF | 3 |  |
| COMPONENT_TYPE_TORSWITCH | 4 |  |
| COMPONENT_TYPE_UMS | 5 |  |
| COMPONENT_TYPE_CDU | 6 |  |



<a name="v1-DiffType"></a>

### DiffType


| Name | Number | Description |
| ---- | ------ | ----------- |
| DIFF_TYPE_UNKNOWN | 0 |  |
| DIFF_TYPE_ONLY_IN_EXPECTED | 1 | In local DB but not in source system |
| DIFF_TYPE_ONLY_IN_ACTUAL | 2 | In source system but not in local DB |
| DIFF_TYPE_DRIFT | 3 | In both but with field differences |



<a name="v1-PowerControlOp"></a>

### PowerControlOp


| Name | Number | Description |
| ---- | ------ | ----------- |
| POWER_CONTROL_OP_UNKNOWN | 0 |  |
| POWER_CONTROL_OP_ON | 1 | Power On |
| POWER_CONTROL_OP_FORCE_ON | 2 |  |
| POWER_CONTROL_OP_OFF | 3 | Power Off

graceful shutdown |
| POWER_CONTROL_OP_FORCE_OFF | 4 |  |
| POWER_CONTROL_OP_RESTART | 5 | Restart (OS level reboot)

graceful restart |
| POWER_CONTROL_OP_FORCE_RESTART | 6 |  |
| POWER_CONTROL_OP_WARM_RESET | 7 | Reset (hardware level) |
| POWER_CONTROL_OP_COLD_RESET | 8 |  |



<a name="v1-TaskExecutorType"></a>

### TaskExecutorType


| Name | Number | Description |
| ---- | ------ | ----------- |
| TASK_EXECUTOR_TYPE_UNKNOWN | 0 |  |
| TASK_EXECUTOR_TYPE_TEMPORAL | 1 |  |



<a name="v1-TaskStatus"></a>

### TaskStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| TASK_STATUS_UNKNOWN | 0 |  |
| TASK_STATUS_PENDING | 1 |  |
| TASK_STATUS_RUNNING | 2 |  |
| TASK_STATUS_COMPLETED | 3 |  |
| TASK_STATUS_FAILED | 4 |  |


 

 


<a name="v1-RLA"></a>

### RLA


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| Version | [VersionRequest](#v1-VersionRequest) | [BuildInfo](#v1-BuildInfo) | What version of RLA is this service running? |
| CreateExpectedRack | [CreateExpectedRackRequest](#v1-CreateExpectedRackRequest) | [CreateExpectedRackResponse](#v1-CreateExpectedRackResponse) |  |
| PatchRack | [PatchRackRequest](#v1-PatchRackRequest) | [PatchRackResponse](#v1-PatchRackResponse) |  |
| GetRackInfoByID | [GetRackInfoByIDRequest](#v1-GetRackInfoByIDRequest) | [GetRackInfoResponse](#v1-GetRackInfoResponse) |  |
| GetRackInfoBySerial | [GetRackInfoBySerialRequest](#v1-GetRackInfoBySerialRequest) | [GetRackInfoResponse](#v1-GetRackInfoResponse) |  |
| GetComponentInfoByID | [GetComponentInfoByIDRequest](#v1-GetComponentInfoByIDRequest) | [GetComponentInfoResponse](#v1-GetComponentInfoResponse) |  |
| GetComponentInfoBySerial | [GetComponentInfoBySerialRequest](#v1-GetComponentInfoBySerialRequest) | [GetComponentInfoResponse](#v1-GetComponentInfoResponse) |  |
| GetListOfRacks | [GetListOfRacksRequest](#v1-GetListOfRacksRequest) | [GetListOfRacksResponse](#v1-GetListOfRacksResponse) |  |
| CreateNVLDomain | [CreateNVLDomainRequest](#v1-CreateNVLDomainRequest) | [CreateNVLDomainResponse](#v1-CreateNVLDomainResponse) |  |
| AttachRacksToNVLDomain | [AttachRacksToNVLDomainRequest](#v1-AttachRacksToNVLDomainRequest) | [.google.protobuf.Empty](#google-protobuf-Empty) |  |
| DetachRacksFromNVLDomain | [DetachRacksFromNVLDomainRequest](#v1-DetachRacksFromNVLDomainRequest) | [.google.protobuf.Empty](#google-protobuf-Empty) |  |
| GetListOfNVLDomains | [GetListOfNVLDomainsRequest](#v1-GetListOfNVLDomainsRequest) | [GetListOfNVLDomainsResponse](#v1-GetListOfNVLDomainsResponse) |  |
| GetRacksForNVLDomain | [GetRacksForNVLDomainRequest](#v1-GetRacksForNVLDomainRequest) | [GetRacksForNVLDomainResponse](#v1-GetRacksForNVLDomainResponse) |  |
| UpgradeFirmware | [UpgradeFirmwareRequest](#v1-UpgradeFirmwareRequest) | [SubmitTaskResponse](#v1-SubmitTaskResponse) |  |
| GetExpectedComponents | [GetExpectedComponentsRequest](#v1-GetExpectedComponentsRequest) | [GetExpectedComponentsResponse](#v1-GetExpectedComponentsResponse) | Components APIs |
| GetActualComponents | [GetActualComponentsRequest](#v1-GetActualComponentsRequest) | [GetActualComponentsResponse](#v1-GetActualComponentsResponse) |  |
| ValidateComponents | [ValidateComponentsRequest](#v1-ValidateComponentsRequest) | [ValidateComponentsResponse](#v1-ValidateComponentsResponse) |  |
| PowerOnRack | [PowerOnRackRequest](#v1-PowerOnRackRequest) | [SubmitTaskResponse](#v1-SubmitTaskResponse) | Power control a rack or a rack&#39;s specified components |
| PowerOffRack | [PowerOffRackRequest](#v1-PowerOffRackRequest) | [SubmitTaskResponse](#v1-SubmitTaskResponse) |  |
| PowerResetRack | [PowerResetRackRequest](#v1-PowerResetRackRequest) | [SubmitTaskResponse](#v1-SubmitTaskResponse) |  |
| ListTasks | [ListTasksRequest](#v1-ListTasksRequest) | [ListTasksResponse](#v1-ListTasksResponse) | Query for tasks |
| GetTasksByIDs | [GetTasksByIDsRequest](#v1-GetTasksByIDsRequest) | [GetTasksByIDsResponse](#v1-GetTasksByIDsResponse) |  |

 



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

