## SMCTF container-provisioner

Container provisioning/allocation HTTP microservice for [SMCTF](https://github.com/nullforu/smctf).

- Port: `8081`

## Concept

In Korean (can be changed later.):

---

이 백엔드 마이크로서비스는 CTF 플랫폼에서 각 유저 개별적인 문제의 Docker compose 환경을 생성하고 관리하는 역할을 위해 아래와 같은 엔드포인트를 제공합니다.
단, 문제의 Docker compose YAML 사양에서 Dockerfile이 있다면 이미지로 빌드하여 <Image:Tag> 형태로 등록되어야하고, 
docker-compose.yaml은 Kubernetes 환경에서 동작할 수 있도록 직접 변환하여 등록해야 합니다.

먼저, 유저와 각 문제의 컨테이너/VM을 나타내는 최소 단위인 "스택(Stack)"을 정의합니다.
각 유저는 문제당 하나의 스택을, 유저 당 최대 N개의 스택을 생성할 수 있습니다. (N은 환경변수로 주입 가능)

이 마이크로서비스는 컨테이너 오케스트레이션 도구로 Kubernetes를 사용하며, Kubernetes Client 라이브러리를 통해 구현합니다.

만약 스케쥴링 할 수 없는 포화 상태라면 스택 생성 요청에 대해 에러를 반환합니다.

---

예시로 문제에서 요구하는 문제 컨테이너의 최소 사양이 256MB의 메모리와 0.5vCPU가 필요하다고 가정합니다.
(최대 512GB 메모리와 2vCPU까지 허용하도록 합니다. 이 값들은 환경변수로 주입 가능)

최대 30명의 유저가 동시에 접속하고, 각 유저가 생성할 수 있는 스택의 최대 개수가 5개로 제한한다면
총 150개의 스택이 필요합니다. 각 스택이 256MB 메모리를 사용한다고 가정하면, 총 메모리 요구량은 약 38.4GB가 되며 CPU 요구량은 75vCPU가 됩니다.

Kubernetes가 컨테이너 오케스트레이션 및 워커 노드 스케쥴링을 담당하기 때문에 백엔드에서 구현할 필요는 없지만, 스택을 관리하는 부분과 Kubernetes 클러스터의 상태, 에러 등은 제대로 관리하고 반환하도록 합니다.

스택의 Pod에서 리소스는 Pod Spec에 정의된 대로 할당되나, 최대 리소스 제한을 초과하지 않도록 검증합니다. 또한 QoS를 위해 리소스 요청량과 제한량을 같도록 강제합니다.

스택에 대한 정보는 AWS DynamoDB에 저장됩니다.

```
{
    "stack_id": "string",          // 스택 고유 ID
    "user_id": "number",           // 유저 고유 ID
    "problem_id": "number",        // 문제 고유 ID
    "pod_id": "string",           // 생성된 Kubernetes Pod의 이름
    "pod_spec": "string",        // 변환된 Kubernetes Pod YAML 스펙 (오로지 Pod 리소스만 포함합니다. 유효성은 이 서비스에서 검증합니다.)
    "target_port": "number",      // 문제 컨테이너가 노출하는 포트 (NodePort로 매핑하기 위함)
    "status": "string",            // 스택 상태 (예: creating, running, stopped, node_deleted 등)
    "created_at": "timestamp",     // 생성 시간
    "updated_at": "timestamp",     // 마지막 업데이트 시간
    // 기타 필요한 정보는 구현 시 고려합니다.
}
```

만약 스케쥴링 할 수 없는 포화 상태라면 스택 생성 요청에 대해 에러를 반환합니다.

## 엔드포인트

- `POST /stacks`
  - 설명: 새로운 스택을 생성합니다.
  - 요청 본문: user_id, problem_id, pod_spec (Kubernetes Pod YAML 스펙) 등
  - 응답: 생성된 스택의 정보 (stack_id, 할당된 node_id 등)

- `GET /stacks/{stack_id}`
  - 설명: 특정 스택의 정보를 조회합니다.
  - 경로 매개변수: stack_id - 조회할 스택의 ID
  - 응답: 스택의 상세 정보

- `GET /stacks/{stack_id}/status`
  - 설명: 특정 스택의 현재 상태를 조회합니다. (이는 메인 백엔드 서비스에서 주기적으로 health 체크를 위해 호출할 수 있습니다.)
  - 경로 매개변수: stack_id - 조회할 스택의 ID
  - 응답: 스택의 현재 상태 (예: creating, running, stopped, node_deleted 등)

- `DELETE /stacks/{stack_id}`
  - 설명: 특정 스택을 삭제합니다. 이때 Kubernetes 클러스터에서 해당 Pod를 종료하고 DynamoDB에서 스택 정보를 삭제합니다. 이미 Pod가 삭제되었더라도 무시하고 DynamoDB에서 스택 정보를 삭제합니다.
  - 경로 매개변수: stack_id - 삭제할 스택의 ID
  - 응답: 성공 여부

- `GET /users/{user_id}/stacks`
  - 설명: 특정 유저가 생성한 모든 스택의 리스트를 조회합니다.
  - 경로 매개변수: user_id - 조회할 유저의 ID
  - 응답: 해당 유저가 생성한 스택들의 리스트

- `GET /stats`
    - 설명: 현재 시스템의 전체 스택 수, 활성 스택 수, 각 노드별 스택 분포 등의 통계 정보를 조회합니다.
    - 응답: 통계 정보 JSON

## Port 할당 방법

기본적으로 모든 스택의 Pod는 서로 격리되며 네트워크적으로 통신되어선 안됩니다. 이를 위해 Kubernetes 네트워크 폴리시를 사용하여 각 Pod 간의 통신을 차단합니다. (이는 인프라 운영자가 따로 설정합니다.)

그리고 모든 스택의 Pod는 NodePort 서비스에 연결되어 사용자는 <NodePublicP:NodePort>를 통해 접근할 수 있어야 합니다.
이때 NodePort는 각 스택이 생성될 때 동적으로 할당됩니다. (기본값: 30000-32767 범위 내에서 랜덤 할당)
충돌을 방지하기 위해 DynamoDB에 현재 사용 중인 NodePort 목록을 저장하고, 새로운 스택 생성 시 사용되지 않는 포트를 할당합니다.
만약 사용 가능한 포트가 없다면 스택 생성 요청에 대해 에러를 반환합니다.

`POST /stacks` 엔드포인트는 오로지 Pod Spec만 제공합니다. NodePort Service는 이 서비스에서 자동으로 생성 및 관리합니다.

스택의 Pod는 무조건 하나만 만들어져야 합니다. 분산되지 않도록 하며, Pod 내에서 여러 컨테이너가 필요한 경우 Pod Spec에 정의된 대로 여러 컨테이너가 포함될 수 있습니다.

# 기타 고려사항

- Kubernetes에서 스택 Pod는 별도의 네임스페이스에 생성됩니다. (예: "stacks") 문제의 스택 Pod에서 NodePort로 노출되는 포트는 오로지 하나여야 합니다. 노출되는 포트는 `POST /stacks` 요청 시 포함된 `target_port` 정보를 바탕으로 합니다.
- 절대적으로 안정성이 중요합니다. Race Condition, 포트 충돌, Deadlock, Leak 등의 문제나 취약점이 없어야 하며 에러 반환을 확실하게 하도록 합니다. (특히 Kubernetes 관련 에러 핸들링)
- 문제의 스택 Pod는 L3/L4 레벨에서 네트워크적으로 격리되어야하며, 문제가 TCP/UDP를 사용할 수 있으므로 방화벽 설정 시 이를 고려해야 합니다.
- 컨테이너 오케스트레이션은 Kubernetes가 담당합니다. 이 마이크로서비스에선 CTF 플랫폼에 맞게 Kubernetes Client 라이브러리를 사용하여 Pod 생성, 삭제, 상태 조회 등을 구현합니다.
- 스택 생성 시 제공된 Kubernetes Pod YAML 스펙의 유효성을 검증합니다. (예: 리소스 제한, 네트워크 설정 등)
- 구현 시 필요에 따라 DB 테이블과 스키마를 수정하거나 추가할 수 있습니다. 이는 구현 시 고려하십시오.
- 각 스택은 TTL을 설정합니다. 최대 2시간(환경변수로 주입 가능) 이후 자동으로 종료되고 삭제됩니다. 이 TTL은 스택 생성 시점부터 계산됩니다. 
- TTL, 고아 Pod 등을 관리하기 위해 주기적으로 백그라운드 작업을 수행하는 스케쥴러가 필요할 수 있습니다. 스케줄링 주기는 기본값 10초이며, 환경변수로 주입 가능합니다.
- 도커 이미지는 빌드되어 ECR이나 Docker hub 등에 미리 푸시되어 있어야 합니다. 이 서비스에서 도커 이미지 빌드는 담당하지 않습니다.
- 안정성을 위해 여유 자원을 항상 확보하도록 합니다. 예를 들어, 20%의 여유 자원이 남아 있어도 스택 생성을 거부합니다. 여유 자원의 비율은 환경변수로 주입 가능합니다.
- 워커노드가 제거되었으나 해당 노드 내에 스택의 Pod가 존재하는 경우 DDB에서 해당 스택 정보를 삭제하고 Kubernetes 클러스터에서 Pod를 종료합니다. 이후 사용자가 `GET /stacks/{stack_id}/status`를 호출하면 스택이 존재하지 않음을 알리는 에러를 반환합니다.
- 보안을 위해 입력시 Pod의 hostNetwork, hostPID, hostIPC, privileged, capabilities, securityContext 등은 허용하지 않습니다. 만약 포함되어 있을 시 에러를 반환합니다. 또한 내부적으로 한번 더 안전하고 호스트에 격리된 보안 컨텍스트를 강제 적용합니다.

이를 구현할 때 말하였던 모든 부분을 반영할 수 있도록 합니다. Go 언어에서 Kubernetes Client 라이브러리를 사용하여 구현합니다.

docs엔 관련 API 문서를 작성하도록 하며, Go 모범 사례를 따르도록, 주석은 복잡한 로직에만 작성하도록 합니다.
