package stack

import (
	"strings"
	"testing"

	"smctf/internal/config"
)

func TestValidatorRejectsHostNetwork(t *testing.T) {
	v := NewValidator(config.StackConfig{})
	_, err := v.ValidatePodSpec(`
apiVersion: v1
kind: Pod
metadata:
  name: bad
spec:
  hostNetwork: true
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 8080
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "100m"
          memory: "64Mi"
`, []PortSpec{{ContainerPort: 8080, Protocol: "TCP"}})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidatorNormalizesResourcesAndHardensWithoutNonRoot(t *testing.T) {
	v := NewValidator(config.StackConfig{})
	res, err := v.ValidatePodSpec(`
apiVersion: v1
kind: Pod
metadata:
  name: good
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 8080
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "200m"
          memory: "128Mi"
`, []PortSpec{{ContainerPort: 8080, Protocol: "TCP"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.RequestedMilli != 200 {
		t.Fatalf("expected 200m, got %d", res.RequestedMilli)
	}

	if strings.Contains(res.SanitizedYAML, "runAsNonRoot: true") {
		t.Fatalf("did not expect runAsNonRoot hardening")
	}

	if !strings.Contains(res.SanitizedYAML, "allowPrivilegeEscalation: false") {
		t.Fatalf("expected allowPrivilegeEscalation hardening")
	}

	if !strings.Contains(res.SanitizedYAML, "privileged: false") {
		t.Fatalf("expected privileged hardening")
	}

	if !strings.Contains(res.SanitizedYAML, "seccompProfile:") {
		t.Fatalf("expected seccompProfile hardening")
	}
}

func TestValidatorRejectsContainerSecurityContext(t *testing.T) {
	v := NewValidator(config.StackConfig{})
	_, err := v.ValidatePodSpec(`
apiVersion: v1
kind: Pod
metadata:
  name: bad-sc
spec:
  containers:
    - name: app
      image: nginx:latest
      securityContext:
        privileged: true
      ports:
        - containerPort: 8080
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "100m"
          memory: "64Mi"
`, []PortSpec{{ContainerPort: 8080, Protocol: "TCP"}})
	if err == nil {
		t.Fatalf("expected security context validation error")
	}
}

func TestValidatorRejectsHostPathVolume(t *testing.T) {
	v := NewValidator(config.StackConfig{})
	_, err := v.ValidatePodSpec(`
apiVersion: v1
kind: Pod
metadata:
  name: bad-hostpath
spec:
  volumes:
    - name: host
      hostPath:
        path: /
  containers:
    - name: app
      image: nginx:latest
      volumeMounts:
        - name: host
          mountPath: /host
      ports:
        - containerPort: 8080
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "100m"
          memory: "64Mi"
`, []PortSpec{{ContainerPort: 8080, Protocol: "TCP"}})
	if err == nil {
		t.Fatalf("expected hostPath validation error")
	}
}

func TestValidatorRejectsProjectedServiceAccountToken(t *testing.T) {
	v := NewValidator(config.StackConfig{})
	_, err := v.ValidatePodSpec(`
apiVersion: v1
kind: Pod
metadata:
  name: bad-satoken
spec:
  volumes:
    - name: tok
      projected:
        sources:
          - serviceAccountToken:
              path: token
              expirationSeconds: 3600
  containers:
    - name: app
      image: nginx:latest
      volumeMounts:
        - name: tok
          mountPath: /var/run/secrets/tok
      ports:
        - containerPort: 8080
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "100m"
          memory: "64Mi"
`, []PortSpec{{ContainerPort: 8080, Protocol: "TCP"}})
	if err == nil {
		t.Fatalf("expected serviceAccountToken projection validation error")
	}
}

func TestValidatorRejectsHostPort(t *testing.T) {
	v := NewValidator(config.StackConfig{})
	_, err := v.ValidatePodSpec(`
apiVersion: v1
kind: Pod
metadata:
  name: bad-hostport
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 8080
          hostPort: 8080
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "100m"
          memory: "64Mi"
`, []PortSpec{{ContainerPort: 8080, Protocol: "TCP"}})
	if err == nil {
		t.Fatalf("expected hostPort validation error")
	}
}

func TestValidatorCountsInitContainerResources(t *testing.T) {
	v := NewValidator(config.StackConfig{})
	res, err := v.ValidatePodSpec(`
apiVersion: v1
kind: Pod
metadata:
  name: init-res
spec:
  initContainers:
    - name: init
      image: busybox:latest
      command: ["sh", "-c", "true"]
      resources:
        requests:
          cpu: "1500m"
          memory: "256Mi"
        limits:
          cpu: "1500m"
          memory: "256Mi"
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 8080
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "100m"
          memory: "64Mi"
`, []PortSpec{{ContainerPort: 8080, Protocol: "TCP"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.RequestedMilli != 1500 {
		t.Fatalf("expected 1500m (init max), got %d", res.RequestedMilli)
	}
}

func TestValidatorRejectsDuplicateTargetPorts(t *testing.T) {
	v := NewValidator(config.StackConfig{})
	_, err := v.ValidatePodSpec(`
apiVersion: v1
kind: Pod
metadata:
  name: dup-targets
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 8080
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "100m"
          memory: "64Mi"
`, []PortSpec{{ContainerPort: 8080, Protocol: "TCP"}, {ContainerPort: 8080, Protocol: "tcp"}})
	if err == nil {
		t.Fatalf("expected duplicate target_port error")
	}
}

func TestValidatorAllowsSubsetOfPodPorts(t *testing.T) {
	v := NewValidator(config.StackConfig{})
	_, err := v.ValidatePodSpec(`
apiVersion: v1
kind: Pod
metadata:
  name: subset-ports
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 8080
        - containerPort: 9090
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "100m"
          memory: "64Mi"
`, []PortSpec{{ContainerPort: 8080, Protocol: "TCP"}})
	if err != nil {
		t.Fatalf("unexpected error for subset targets: %v", err)
	}
}

func TestValidatorRejectsMissingProtocol(t *testing.T) {
	v := NewValidator(config.StackConfig{})
	_, err := v.ValidatePodSpec(`
apiVersion: v1
kind: Pod
metadata:
  name: missing-proto
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 8080
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "100m"
          memory: "64Mi"
`, []PortSpec{{ContainerPort: 8080}})
	if err == nil {
		t.Fatalf("expected missing protocol error")
	}
}

func TestValidatorRejectsInvalidProtocol(t *testing.T) {
	v := NewValidator(config.StackConfig{})
	_, err := v.ValidatePodSpec(`
apiVersion: v1
kind: Pod
metadata:
  name: invalid-proto
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 8080
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "100m"
          memory: "64Mi"
`, []PortSpec{{ContainerPort: 8080, Protocol: "SCTP"}})
	if err == nil {
		t.Fatalf("expected invalid protocol error")
	}
}

func TestValidatorAcceptsUDPProtocol(t *testing.T) {
	v := NewValidator(config.StackConfig{})
	_, err := v.ValidatePodSpec(`
apiVersion: v1
kind: Pod
metadata:
  name: udp-proto
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 8080
          protocol: UDP
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "100m"
          memory: "64Mi"
`, []PortSpec{{ContainerPort: 8080, Protocol: "UDP"}})
	if err != nil {
		t.Fatalf("unexpected udp protocol error: %v", err)
	}
}
