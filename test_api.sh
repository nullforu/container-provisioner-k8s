#!/usr/bin/env bash
set -euo pipefail

API_BASE="${API_BASE:-http://localhost:8081}"
STACK_NAMESPACE="${STACK_NAMESPACE:-stacks}"
USER_ID="${USER_ID:-91001}"
PROBLEM_ID="${PROBLEM_ID:-81001}"
TARGET_PORT="${TARGET_PORT:-5000}"
KUBECTL_TIMEOUT_SECONDS="${KUBECTL_TIMEOUT_SECONDS:-60}"

STACK_ID=""
POD_ID=""
SERVICE_NAME=""
NODE_PORT=""

log() {
  printf '[test_api] %s\n' "$*"
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "required command not found: $cmd" >&2
    exit 1
  fi
}

request() {
  local method="$1"
  local url="$2"
  local body="${3:-}"

  if [[ -n "$body" ]]; then
    curl -sS -X "$method" "$url" \
      -H "Content-Type: application/json" \
      -d "$body" \
      -w $'\n%{http_code}'
  else
    curl -sS -X "$method" "$url" \
      -w $'\n%{http_code}'
  fi
}

split_response() {
  local response="$1"
  RESP_CODE="$(printf '%s' "$response" | tail -n1)"
  RESP_BODY="$(printf '%s' "$response" | sed '$d')"
}

assert_status() {
  local expected="$1"
  if [[ "$RESP_CODE" != "$expected" ]]; then
    echo "unexpected status: got=$RESP_CODE expected=$expected" >&2
    echo "response body: $RESP_BODY" >&2
    exit 1
  fi
}

cleanup() {
  if [[ -n "$STACK_ID" ]]; then
    log "cleanup: DELETE /stacks/$STACK_ID"
    request DELETE "$API_BASE/stacks/$STACK_ID" >/dev/null || true
  fi
}

wait_stack_status_running() {
  local deadline
  deadline="$((SECONDS + KUBECTL_TIMEOUT_SECONDS))"

  while (( SECONDS < deadline )); do
    split_response "$(request GET "$API_BASE/stacks/$STACK_ID/status")"

    if [[ "$RESP_CODE" == "200" ]]; then
      local stack_status
      stack_status="$(printf '%s' "$RESP_BODY" | jq -r '.status // empty')"

      if [[ "$stack_status" == "running" ]]; then
        return 0
      fi
    fi

    sleep 2
  done

  echo "stack status did not become running within timeout" >&2
  split_response "$(request GET "$API_BASE/stacks/$STACK_ID/status")"
  echo "last status response: code=$RESP_CODE body=$RESP_BODY" >&2
  exit 1
}

wait_kubectl_gone() {
  local kind="$1"
  local name="$2"
  local deadline
  deadline="$((SECONDS + KUBECTL_TIMEOUT_SECONDS))"

  while (( SECONDS < deadline )); do
    if ! kubectl get "$kind" "$name" -n "$STACK_NAMESPACE" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  echo "$kind/$name still exists in namespace=$STACK_NAMESPACE" >&2
  return 1
}

main() {
#   trap cleanup EXIT

  require_cmd curl
  require_cmd jq
  require_cmd kubectl

  log "checking server health: $API_BASE/healthz"
  split_response "$(request GET "$API_BASE/healthz")"
  assert_status "200"
  if [[ "$(printf '%s' "$RESP_BODY" | jq -r '.status // empty')" != "ok" ]]; then
    echo "healthz payload invalid: $RESP_BODY" >&2
    exit 1
  fi

  log "checking kubectl connectivity"
  kubectl version --request-timeout='10s' >/dev/null

  POD_SPEC="$(cat <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: test-challenge
spec:
  containers:
    - name: app
      image: nginx:stable
      ports:
        - containerPort: 5000
          protocol: TCP
      resources:
        requests:
          cpu: "100m"
          memory: "128Mi"
        limits:
          cpu: "100m"
          memory: "128Mi"
YAML
)"

  CREATE_PAYLOAD="$(jq -n \
    --argjson user_id "$USER_ID" \
    --argjson problem_id "$PROBLEM_ID" \
    --argjson target_port "$TARGET_PORT" \
    --arg pod_spec "$POD_SPEC" \
    '{user_id:$user_id, problem_id:$problem_id, target_port:$target_port, pod_spec:$pod_spec}')"

  log "POST /stacks"
  split_response "$(request POST "$API_BASE/stacks" "$CREATE_PAYLOAD")"
  assert_status "201"

  STACK_ID="$(printf '%s' "$RESP_BODY" | jq -r '.stack_id')"
  POD_ID="$(printf '%s' "$RESP_BODY" | jq -r '.pod_id')"
  SERVICE_NAME="$(printf '%s' "$RESP_BODY" | jq -r '.service_name')"
  NODE_PORT="$(printf '%s' "$RESP_BODY" | jq -r '.node_port')"

  if [[ -z "$STACK_ID" || "$STACK_ID" == "null" ]]; then
    echo "missing stack_id in create response: $RESP_BODY" >&2
    exit 1
  fi

  log "GET /stacks/$STACK_ID"
  split_response "$(request GET "$API_BASE/stacks/$STACK_ID")"
  assert_status "200"

  log "GET /stacks/$STACK_ID/status (wait until running)"
  wait_stack_status_running

  log "GET /users/$USER_ID/stacks"
  split_response "$(request GET "$API_BASE/users/$USER_ID/stacks")"
  assert_status "200"
  if ! printf '%s' "$RESP_BODY" | jq -e --arg sid "$STACK_ID" '.stacks[] | select(.stack_id == $sid)' >/dev/null; then
    echo "created stack is not listed for user: $RESP_BODY" >&2
    exit 1
  fi

  log "GET /stats"
  split_response "$(request GET "$API_BASE/stats")"
  assert_status "200"

  log "kubectl verify pod/service existence"
  kubectl get pod "$POD_ID" -n "$STACK_NAMESPACE" -o name >/dev/null
  kubectl get svc "$SERVICE_NAME" -n "$STACK_NAMESPACE" -o name >/dev/null

  K8S_NODE_PORT="$(kubectl get svc "$SERVICE_NAME" -n "$STACK_NAMESPACE" -o jsonpath='{.spec.ports[0].nodePort}')"
  if [[ "$K8S_NODE_PORT" != "$NODE_PORT" ]]; then
    echo "nodePort mismatch api=$NODE_PORT kubectl=$K8S_NODE_PORT" >&2
    exit 1
  fi

#   log "DELETE /stacks/$STACK_ID"
#   split_response "$(request DELETE "$API_BASE/stacks/$STACK_ID")"
#   assert_status "200"

#   log "verify deleted stack returns 404"
#   split_response "$(request GET "$API_BASE/stacks/$STACK_ID")"
#   assert_status "404"

#   split_response "$(request GET "$API_BASE/stacks/$STACK_ID/status")"
#   assert_status "404"

#   log "kubectl verify pod/service deletion"
#   wait_kubectl_gone pod "$POD_ID"
#   wait_kubectl_gone svc "$SERVICE_NAME"

#   STACK_ID=""
#   POD_ID=""
#   SERVICE_NAME=""

#   log "GET /stats (post-delete)"
#   split_response "$(request GET "$API_BASE/stats")"
#   assert_status "200"

#   log "all endpoint + kubectl checks passed"
}

main "$@"
