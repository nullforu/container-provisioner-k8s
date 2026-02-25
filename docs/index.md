---
title: Home
nav_order: 1
---

Base URL: `http://localhost:8081`

## Authentication

All endpoints require an API key when API key auth is enabled (default). You can provide it via:

- `X-API-KEY` header
- `api_key` query parameter

```bash
curl -H "X-API-KEY: <your-api-key>" http://localhost:8081/healthz
curl "http://localhost:8081/healthz?api_key=<your-api-key>"
```

## Health

- `GET /healthz`
- Response

```json
{
    "status": "ok"
}
```

## Stats

- `GET /stats`
- Success: `200 OK`

**Response**

```json
{
    "total_stacks": 7,
    "active_stacks": 7,
    "node_distribution": {
        "dev-worker": 3,
        "dev-worker2": 4
    },
    "used_node_ports": 7,
    "reserved_cpu_milli": 700,
    "reserved_memory_bytes": 939524096
}
```

## Stack APIs

### Create Stack

- `POST /stacks`
- Body

```json
{
    "target_port": [
        {
            "container_port": 80,
            "protocol": "TCP"
        }
    ],
    "pod_spec": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: challenge\nspec:\n  containers:\n    - name: app\n      image: nginx:stable\n      ports:\n        - containerPort: 80\n          protocol: TCP\n      resources:\n        requests:\n          cpu: \"100m\"\n          memory: \"128Mi\"\n        limits:\n          cpu: \"100m\"\n          memory: \"128Mi\""
}
```

- Success:
    - `201 Created`
- Failure:
    - `400 Bad Request` (invalid pod spec)
    - `400 Bad Request` (LimitRange violation)
    - `503 Service Unavailable` (no available nodeport)
    - `503 Service Unavailable` (ResourceQuota violation)

**Response**

```json
{
    "stack_id": "stack-716b6384dd477b0b",
    "pod_id": "stack-716b6384dd477b0b",
    "namespace": "stacks",
    "node_id": "dev-worker2",
    "node_public_ip": "12.34.56.78",
    "pod_spec": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: challenge\nspec:\n  automountServiceAccountToken: false\n  containers:\n  - image: nginx:stable\n    name: app\n    ports:\n    - containerPort: 80\n      protocol: TCP\n    resources:\n      limits:\n        cpu: 100m\n        memory: 128Mi\n      requests:\n        cpu: 100m\n        memory: 128Mi\n    securityContext:\n      allowPrivilegeEscalation: false\n      privileged: false\n      seccompProfile:\n        type: RuntimeDefault\n  enableServiceLinks: false\n  restartPolicy: Never\n  securityContext:\n    seccompProfile:\n      type: RuntimeDefault\nstatus: {}\n",
    "ports": [
        {
            "container_port": 80,
            "protocol": "TCP",
            "node_port": 31538
        }
    ],
    "service_name": "svc-stack-716b6384dd477b0b",
    "status": "creating",
    "ttl_expires_at": "2026-02-10T04:02:26.535664Z",
    "created_at": "2026-02-10T02:02:26.535664Z",
    "updated_at": "2026-02-10T02:02:26.535664Z",
    "requested_cpu_milli": 100,
    "requested_memory_bytes": 134217728
}
```

### List All Stacks

- `GET /stacks`
- Success: `200 OK`

**Response**

```json
{
    "stacks": [
        {
            "stack_id": "stack-716b6384dd477b0b",
            "pod_id": "stack-716b6384dd477b0b",
            "namespace": "stacks",
            "node_id": "dev-worker2",
            "node_public_ip": "12.34.56.78",
            "pod_spec": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: challenge\nspec:\n  automountServiceAccountToken: false\n  containers:\n  - image: nginx:stable\n    name: app\n    ports:\n    - containerPort: 80\n      protocol: TCP\n    resources:\n      limits:\n        cpu: 100m\n        memory: 128Mi\n      requests:\n        cpu: 100m\n        memory: 128Mi\n    securityContext:\n      allowPrivilegeEscalation: false\n      privileged: false\n      seccompProfile:\n        type: RuntimeDefault\n  enableServiceLinks: false\n  restartPolicy: Never\n  securityContext:\n    seccompProfile:\n      type: RuntimeDefault\nstatus: {}\n",
            "ports": [
                {
                    "container_port": 80,
                    "protocol": "TCP",
                    "node_port": 31538
                }
            ],
            "service_name": "svc-stack-716b6384dd477b0b",
            "status": "running",
            "ttl_expires_at": "2026-02-10T04:02:26.535664Z",
            "created_at": "2026-02-10T02:02:26.535664Z",
            "updated_at": "2026-02-10T02:06:33.16031Z",
            "requested_cpu_milli": 100,
            "requested_memory_bytes": 134217728
        }
    ]
}
```

### Get Stack

- `GET /stacks/{stack_id}`
- Success: `200 OK`

**Response**

```json
{
    "stack_id": "stack-716b6384dd477b0b",
    "pod_id": "stack-716b6384dd477b0b",
    "namespace": "stacks",
    "node_id": "dev-worker2",
    "node_public_ip": "12.34.56.78",
    "pod_spec": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: challenge\nspec:\n  automountServiceAccountToken: false\n  containers:\n  - image: nginx:stable\n    name: app\n    ports:\n    - containerPort: 80\n      protocol: TCP\n    resources:\n      limits:\n        cpu: 100m\n        memory: 128Mi\n      requests:\n        cpu: 100m\n        memory: 128Mi\n    securityContext:\n      allowPrivilegeEscalation: false\n      privileged: false\n      seccompProfile:\n        type: RuntimeDefault\n  enableServiceLinks: false\n  restartPolicy: Never\n  securityContext:\n    seccompProfile:\n      type: RuntimeDefault\nstatus: {}\n",
    "ports": [
        {
            "container_port": 80,
            "protocol": "TCP",
            "node_port": 31538
        }
    ],
    "service_name": "svc-stack-716b6384dd477b0b",
    "status": "running",
    "ttl_expires_at": "2026-02-10T04:02:26.535664Z",
    "created_at": "2026-02-10T02:02:26.535664Z",
    "updated_at": "2026-02-10T02:07:29.530829Z",
    "requested_cpu_milli": 100,
    "requested_memory_bytes": 134217728
}
```

### Get Stack Status

- `GET /stacks/{stack_id}/status`
- Success: `200 OK`

**Response**

```json
{
    "stack_id": "stack-716b6384dd477b0b",
    "status": "running",
    "ttl": "2026-02-10T04:02:26.535664Z",
    "ports": [
        {
            "container_port": 80,
            "protocol": "TCP",
            "node_port": 31538
        }
    ],
    "node_public_ip": "12.34.56.78"
}
```

### Delete Stack

- `DELETE /stacks/{stack_id}`
- Success:
    - `200 OK`
- Failure:
    - `404 Not Found` (stack not found)

**Response**

```json
{
    "deleted": true,
    "stack_id": "stack-716b6384dd477b0b"
}
```

## Stack statuses

- `creating`: the stack is being created. The pod may not be running yet.
- `running`: the stack is running and ready to accept traffic.
- `stopped`: the stack has been stopped by the user. The pod has been deleted.
- `failed`: the stack failed to start. Check the pod events/logs for more details.
- `node_deleted`: the node where the stack was running has been deleted. The stack is no longer accessible.

## Error codes

- `400`: invalid request body / pod spec validation error
- `400`: Kubernetes `LimitRange` violation
- `404`: stack not found
- `503`: cluster saturation, no available nodeport
- `503`: Kubernetes `ResourceQuota` violation
- `500`: internal server error
