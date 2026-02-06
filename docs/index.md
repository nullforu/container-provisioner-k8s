---
title: Home
nav_order: 1
---

# SMCTF Container Provisioner API Documentation

Base URL: `http://localhost:8081`

## Health

- `GET /healthz`
- Response

```json
{
  "status": "ok"
}
```

## Stack APIs

### Create Stack

- `POST /stacks`
- Body

```json
{
  "target_port": 5000,
  "pod_spec": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: problem-1001\nspec:\n  containers:\n    - name: app\n      image: ghcr.io/example/problem:latest\n      ports:\n        - containerPort: 5000\n          protocol: TCP\n      resources:\n        requests:\n          cpu: \"500m\"\n          memory: \"256Mi\"\n        limits:\n          cpu: \"500m\"\n          memory: \"256Mi\"\n"
}
```

- Success: `201 Created`
- Response includes `node_public_ip` (null when the node has no public IP).

### List All Stacks

- `GET /stacks`
- Success: `200 OK`
- Each stack includes `node_public_ip` (null when the node has no public IP).

### Get Stack

- `GET /stacks/{stack_id}`
- Success: `200 OK`
- Response includes `node_public_ip` (null when the node has no public IP).

### Get Stack Status

- `GET /stacks/{stack_id}/status`
- Success: `200 OK`
- Response fields:
  - `stack_id`
  - `status`
  - `ttl`
  - `node_port`
  - `target_port`
  - `node_public_ip`

### Delete Stack

- `DELETE /stacks/{stack_id}`
- Success: `200 OK`

### Stats

- `GET /stats`
- Success: `200 OK`

## Error codes

- `400`: invalid request body / pod spec validation error
- `400`: Kubernetes `LimitRange` 초과 (예: 컨테이너별 최대/최소 리소스 위반)
- `404`: stack not found
- `503`: cluster saturation, no available nodeport
- `503`: Kubernetes `ResourceQuota` 초과
- `500`: internal server error

## Validation and safety rules

- Pod spec must be a single `kind: Pod` resource.
- Exactly one exposed container port must exist, and it must equal `target_port`.
- `hostNetwork`, `hostPID`, `hostIPC`, input `securityContext`, `privileged`, capabilities escalation are forbidden.
- QoS guarantee: requests and limits are normalized to equal values internally.
- Max per-stack resource limits are enforced (`STACK_MAX_CPU`, `STACK_MAX_MEMORY`).
- NodePort is allocated from `STACK_NODEPORT_MIN..STACK_NODEPORT_MAX` without collision.
- Scheduler removes TTL-expired stacks, stacks with missing Pod/Service, and orphaned stacks on deleted nodes.

## Runtime dependencies

- Kubernetes API access (`client-go`)
- AWS DynamoDB table with keys:
  - partition key: `pk` (String)
  - sort key: `sk` (String)
  - GSI `gsi1` (`gsi1pk`, `gsi1sk`) for listing stacks

## Key Environment Variables

- `STACK_TTL`
- `STACK_SCHEDULER_INTERVAL`
- `STACK_NODEPORT_MIN`, `STACK_NODEPORT_MAX`
- `STACK_PORT_LOCK_TTL`
- `STACK_RESOURCE_RESERVE_RATIO`
- `STACK_SCHEDULING_TIMEOUT`
- `STACK_REQUIRE_INGRESS_NETWORK_POLICY`
- `DDB_STACK_TABLE`, `AWS_REGION`
