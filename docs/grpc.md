---
title: gRPC
nav_order: 2
---

# gRPC API

Service: `stack.v1.StackService`

Default address: `localhost:9090` (config: `GRPC_ADDR`)

Authentication: if API key auth is enabled, send metadata key `x-api-key` with the same value used for REST.

Example (grpcurl):

```bash
grpcurl -plaintext -H "x-api-key: <your-api-key>" localhost:9090 stack.v1.StackService/Healthz
```

## Methods

### Healthz

- RPC: `Healthz(HealthzRequest) returns (HealthzResponse)`
- Description: health check

**Request**

```proto
message HealthzRequest {}
```

**Response**

```proto
message HealthzResponse {
  string status = 1;
}
```

### CreateStack

- RPC: `CreateStack(CreateStackRequest) returns (CreateStackResponse)`
- Description: create a stack

**Request**

```proto
message CreateStackRequest {
  string pod_spec = 1;
  repeated PortSpec target_ports = 2;
}
```

**Response**

```proto
message CreateStackResponse {
  Stack stack = 1;
}
```

### GetStack

- RPC: `GetStack(GetStackRequest) returns (GetStackResponse)`
- Description: get a stack by ID

**Request**

```proto
message GetStackRequest {
  string stack_id = 1;
}
```

**Response**

```proto
message GetStackResponse {
  Stack stack = 1;
}
```

### GetStackStatusSummary

- RPC: `GetStackStatusSummary(GetStackStatusSummaryRequest) returns (GetStackStatusSummaryResponse)`
- Description: get stack status summary

**Request**

```proto
message GetStackStatusSummaryRequest {
  string stack_id = 1;
}
```

**Response**

```proto
message GetStackStatusSummaryResponse {
  StackStatusSummary summary = 1;
}
```

### DeleteStack

- RPC: `DeleteStack(DeleteStackRequest) returns (DeleteStackResponse)`
- Description: delete a stack by ID

**Request**

```proto
message DeleteStackRequest {
  string stack_id = 1;
}
```

**Response**

```proto
message DeleteStackResponse {
  bool deleted = 1;
  string stack_id = 2;
}
```

### ListStacks

- RPC: `ListStacks(ListStacksRequest) returns (ListStacksResponse)`
- Description: list stacks

**Request**

```proto
message ListStacksRequest {}
```

**Response**

```proto
message ListStacksResponse {
  repeated Stack stacks = 1;
}
```

### CreateBatchDeleteJob

- RPC: `CreateBatchDeleteJob(CreateBatchDeleteJobRequest) returns (CreateBatchDeleteJobResponse)`
- Description: create batch delete job

**Request**

```proto
message CreateBatchDeleteJobRequest {
  repeated string stack_ids = 1;
}
```

**Response**

```proto
message CreateBatchDeleteJobResponse {
  string job_id = 1;
}
```

### GetBatchDeleteJob

- RPC: `GetBatchDeleteJob(GetBatchDeleteJobRequest) returns (GetBatchDeleteJobResponse)`
- Description: get batch delete job by ID

**Request**

```proto
message GetBatchDeleteJobRequest {
  string job_id = 1;
}
```

**Response**

```proto
message GetBatchDeleteJobResponse {
  BatchDeleteJob job = 1;
}
```

### GetStats

- RPC: `GetStats(GetStatsRequest) returns (GetStatsResponse)`
- Description: get cluster stats

**Request**

```proto
message GetStatsRequest {}
```

**Response**

```proto
message GetStatsResponse {
  Stats stats = 1;
}
```

## Messages

### Stack

```proto
message Stack {
  string stack_id = 1;
  string pod_id = 2;
  string namespace = 3;
  string node_id = 4;
  optional string node_public_ip = 5;
  string pod_spec = 6;
  repeated PortMapping ports = 7;
  string service_name = 8;
  Status status = 9;
  google.protobuf.Timestamp ttl_expires_at = 10;
  google.protobuf.Timestamp created_at = 11;
  google.protobuf.Timestamp updated_at = 12;
  int64 requested_cpu_milli = 13;
  int64 requested_memory_bytes = 14;
  repeated PortSpec target_ports = 15;
}
```

### StackStatusSummary

```proto
message StackStatusSummary {
  string stack_id = 1;
  Status status = 2;
  google.protobuf.Timestamp ttl = 3;
  repeated PortMapping ports = 4;
  optional string node_public_ip = 5;
  repeated PortSpec target_ports = 6;
}
```

### PortSpec

```proto
message PortSpec {
  int32 container_port = 1;
  string protocol = 2;
}
```

### PortMapping

```proto
message PortMapping {
  int32 container_port = 1;
  string protocol = 2;
  int32 node_port = 3;
}
```

### BatchDeleteJob

```proto
message BatchDeleteJob {
  string job_id = 1;
  JobStatus status = 2;
  int32 total = 3;
  int32 deleted = 4;
  int32 not_found = 5;
  int32 failed = 6;
  repeated JobError errors = 7;
  google.protobuf.Timestamp created_at = 8;
  google.protobuf.Timestamp updated_at = 9;
}
```

### JobError

```proto
message JobError {
  string stack_id = 1;
  string error = 2;
}
```

### Stats

```proto
message Stats {
  int32 total_stacks = 1;
  int32 active_stacks = 2;
  map<string, int32> node_distribution = 3;
  int32 used_node_ports = 4;
  int64 reserved_cpu_milli = 5;
  int64 reserved_memory_bytes = 6;
}
```

## Enums

### Status

```proto
enum Status {
  STATUS_UNSPECIFIED = 0;
  STATUS_CREATING = 1;
  STATUS_RUNNING = 2;
  STATUS_STOPPED = 3;
  STATUS_FAILED = 4;
  STATUS_NODE_DELETED = 5;
}
```

### JobStatus

```proto
enum JobStatus {
  JOB_STATUS_UNSPECIFIED = 0;
  JOB_STATUS_QUEUED = 1;
  JOB_STATUS_RUNNING = 2;
  JOB_STATUS_COMPLETED = 3;
  JOB_STATUS_FAILED = 4;
}
```

## Errors

gRPC errors map from the same domain errors used in REST:

- `InvalidArgument`: invalid input or invalid pod spec
- `NotFound`: stack not found
- `Unavailable`: no available nodeport or cluster saturated
- `Internal`: unexpected server error
