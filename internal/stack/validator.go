package stack

import (
	"fmt"
	"strings"

	"smctf/internal/config"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	sigsyaml "sigs.k8s.io/yaml"
)

type Validator struct {
	cfg config.StackConfig
}

func NewValidator(cfg config.StackConfig) *Validator {
	return &Validator{cfg: cfg}
}

type ValidationResult struct {
	SanitizedYAML  string
	RequestedMilli int64
	RequestedBytes int64
	TargetPorts    []PortSpec
}

const maxTargetPorts = 24

func (v *Validator) ValidatePodSpec(raw string, targetPorts []PortSpec) (ValidationResult, error) {
	if strings.TrimSpace(raw) == "" {
		return ValidationResult{}, fmt.Errorf("%w: pod_spec is required", ErrPodSpecInvalid)
	}

	if len(targetPorts) == 0 {
		return ValidationResult{}, fmt.Errorf("%w: target_port is required", ErrInvalidInput)
	}

	if len(targetPorts) > maxTargetPorts {
		return ValidationResult{}, fmt.Errorf("%w: target_port exceeds limit (max %d)", ErrInvalidInput, maxTargetPorts)
	}

	normalizedTargets := make([]PortSpec, 0, len(targetPorts))
	targetSet := make(map[string]struct{}, len(targetPorts))
	for _, tp := range targetPorts {
		if tp.ContainerPort < 1 || tp.ContainerPort > 65535 {
			return ValidationResult{}, fmt.Errorf("%w: target_port is out of range", ErrInvalidInput)
		}

		proto, err := normalizeProtocol(tp.Protocol)
		if err != nil {
			return ValidationResult{}, err
		}

		key := portKey(tp.ContainerPort, proto)
		if _, exists := targetSet[key]; exists {
			return ValidationResult{}, fmt.Errorf("%w: duplicate target_port entry", ErrInvalidInput)
		}

		targetSet[key] = struct{}{}
		normalizedTargets = append(normalizedTargets, PortSpec{ContainerPort: tp.ContainerPort, Protocol: proto})
	}

	var pod corev1.Pod
	if err := sigsyaml.Unmarshal([]byte(raw), &pod); err != nil {
		return ValidationResult{}, fmt.Errorf("%w: yaml parse failed", ErrPodSpecInvalid)
	}

	if !strings.EqualFold(pod.Kind, "Pod") {
		return ValidationResult{}, fmt.Errorf("%w: kind must be Pod", ErrPodSpecInvalid)
	}

	if pod.Spec.HostNetwork || pod.Spec.HostPID || pod.Spec.HostIPC {
		return ValidationResult{}, fmt.Errorf("%w: hostNetwork/hostPID/hostIPC are forbidden", ErrPodSpecInvalid)
	}

	if pod.Spec.SecurityContext != nil {
		return ValidationResult{}, fmt.Errorf("%w: pod securityContext is forbidden in input", ErrPodSpecInvalid)
	}

	if pod.Spec.ServiceAccountName != "" || pod.Spec.DeprecatedServiceAccount != "" {
		return ValidationResult{}, fmt.Errorf("%w: serviceAccount is forbidden in input", ErrPodSpecInvalid)
	}

	if pod.Spec.NodeName != "" || pod.Spec.RuntimeClassName != nil {
		return ValidationResult{}, fmt.Errorf("%w: nodeName/runtimeClassName are forbidden in input", ErrPodSpecInvalid)
	}

	if len(pod.Spec.EphemeralContainers) > 0 {
		return ValidationResult{}, fmt.Errorf("%w: ephemeralContainers are forbidden", ErrPodSpecInvalid)
	}

	if err := validateVolumesNoHostAccess(pod.Spec.Volumes); err != nil {
		return ValidationResult{}, err
	}

	if len(pod.Spec.Containers) == 0 {
		return ValidationResult{}, fmt.Errorf("%w: at least one container is required", ErrPodSpecInvalid)
	}

	var sumMilli int64
	var sumBytes int64
	var initMaxMilli int64
	var initMaxBytes int64
	portCount := 0
	podPortSet := make(map[string]struct{})

	for i := range pod.Spec.InitContainers {
		c := &pod.Spec.InitContainers[i]
		if err := validateContainerBasics(c); err != nil {
			return ValidationResult{}, err
		}

		if len(c.Ports) > 0 {
			return ValidationResult{}, fmt.Errorf("%w: initContainer ports are forbidden", ErrPodSpecInvalid)
		}

		cpuMilli, memBytes, err := normalizeAndValidateResources(c.Resources)
		if err != nil {
			return ValidationResult{}, err
		}
		c.Resources = buildEqualResources(cpuMilli, memBytes)

		if cpuMilli > initMaxMilli {
			initMaxMilli = cpuMilli
		}

		if memBytes > initMaxBytes {
			initMaxBytes = memBytes
		}
	}

	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		if err := validateContainerBasics(c); err != nil {
			return ValidationResult{}, err
		}

		cpuMilli, memBytes, err := normalizeAndValidateResources(c.Resources)
		if err != nil {
			return ValidationResult{}, err
		}

		c.Resources = buildEqualResources(cpuMilli, memBytes)
		sumMilli += cpuMilli
		sumBytes += memBytes

		for _, p := range c.Ports {
			if p.ContainerPort < 1 || p.ContainerPort > 65535 {
				return ValidationResult{}, fmt.Errorf("%w: invalid container port", ErrPodSpecInvalid)
			}

			if p.HostPort != 0 || p.HostIP != "" {
				return ValidationResult{}, fmt.Errorf("%w: hostPort/hostIP are forbidden", ErrPodSpecInvalid)
			}

			if p.Protocol != "" && p.Protocol != corev1.ProtocolTCP && p.Protocol != corev1.ProtocolUDP {
				return ValidationResult{}, fmt.Errorf("%w: protocol must be TCP or UDP", ErrPodSpecInvalid)
			}

			portCount++
			proto := string(p.Protocol)
			if proto == "" {
				proto = string(corev1.ProtocolTCP)
			}

			key := portKey(int(p.ContainerPort), proto)
			podPortSet[key] = struct{}{}
		}
	}

	if portCount == 0 {
		return ValidationResult{}, fmt.Errorf("%w: at least one exposed container port is required", ErrPodSpecInvalid)
	}

	for _, tp := range normalizedTargets {
		key := portKey(tp.ContainerPort, tp.Protocol)
		if _, ok := podPortSet[key]; !ok {
			return ValidationResult{}, fmt.Errorf("%w: target_port must exist in container ports", ErrPodSpecInvalid)
		}
	}

	reqMilli := max64(sumMilli, initMaxMilli)
	reqBytes := max64(sumBytes, initMaxBytes)

	if reqMilli <= 0 || reqBytes <= 0 {
		return ValidationResult{}, fmt.Errorf("%w: resources are required", ErrPodSpecInvalid)
	}

	pod.Spec.RestartPolicy = corev1.RestartPolicyNever
	pod.Spec.AutomountServiceAccountToken = boolPtr(false)
	pod.Spec.EnableServiceLinks = boolPtr(false)

	pod.Spec.SecurityContext = &corev1.PodSecurityContext{
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}

	for i := range pod.Spec.InitContainers {
		pod.Spec.InitContainers[i].SecurityContext = hardenedContainerSecurityContext()
	}

	for i := range pod.Spec.Containers {
		pod.Spec.Containers[i].SecurityContext = hardenedContainerSecurityContext()
	}

	sanitized, err := sigsyaml.Marshal(&pod)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("%w: yaml marshal failed", ErrPodSpecInvalid)
	}

	return ValidationResult{
		SanitizedYAML:  string(sanitized),
		RequestedMilli: reqMilli,
		RequestedBytes: reqBytes,
		TargetPorts:    normalizedTargets,
	}, nil
}

func normalizeProtocol(proto string) (string, error) {
	upper := strings.ToUpper(strings.TrimSpace(proto))
	if upper == "" {
		return "", fmt.Errorf("%w: protocol is required", ErrInvalidInput)
	}

	if upper != "TCP" && upper != "UDP" {
		return "", fmt.Errorf("%w: protocol must be TCP or UDP", ErrInvalidInput)

	}

	return upper, nil
}

func portKey(port int, proto string) string {
	return fmt.Sprintf("%d/%s", port, strings.ToUpper(proto))
}

func validateContainerBasics(c *corev1.Container) error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("%w: container name is required", ErrPodSpecInvalid)
	}

	if strings.TrimSpace(c.Image) == "" {
		return fmt.Errorf("%w: container image is required", ErrPodSpecInvalid)
	}

	if c.SecurityContext != nil {
		return fmt.Errorf("%w: container securityContext is forbidden in input", ErrPodSpecInvalid)
	}

	return nil
}

func validateVolumesNoHostAccess(vols []corev1.Volume) error {
	for _, v := range vols {
		if v.HostPath != nil {
			return fmt.Errorf("%w: hostPath volume is forbidden", ErrPodSpecInvalid)
		}

		if v.Projected != nil {
			for _, src := range v.Projected.Sources {
				if src.ServiceAccountToken != nil {
					return fmt.Errorf("%w: projected serviceAccountToken is forbidden", ErrPodSpecInvalid)
				}
			}
		}
	}

	return nil
}

func hardenedContainerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		Privileged:               boolPtr(false),
		AllowPrivilegeEscalation: boolPtr(false),
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}

func normalizeAndValidateResources(r corev1.ResourceRequirements) (int64, int64, error) {
	cpuReq := getMilli(r.Requests, corev1.ResourceCPU)
	cpuLim := getMilli(r.Limits, corev1.ResourceCPU)
	memReq := getBytes(r.Requests, corev1.ResourceMemory)
	memLim := getBytes(r.Limits, corev1.ResourceMemory)

	cpuMilli := max64(cpuReq, cpuLim)
	memBytes := max64(memReq, memLim)
	if cpuMilli <= 0 || memBytes <= 0 {
		return 0, 0, fmt.Errorf("%w: request/limit must be set", ErrPodSpecInvalid)
	}

	return cpuMilli, memBytes, nil
}

func buildEqualResources(cpuMilli, memBytes int64) corev1.ResourceRequirements {
	cpuQ := *resource.NewMilliQuantity(cpuMilli, resource.DecimalSI)
	memQ := *resource.NewQuantity(memBytes, resource.BinarySI)

	list := corev1.ResourceList{
		corev1.ResourceCPU:    cpuQ,
		corev1.ResourceMemory: memQ,
	}

	return corev1.ResourceRequirements{
		Requests: list,
		Limits:   list,
	}
}

func getMilli(list corev1.ResourceList, name corev1.ResourceName) int64 {
	q, ok := list[name]
	if !ok {
		return 0
	}

	return q.MilliValue()
}

func getBytes(list corev1.ResourceList, name corev1.ResourceName) int64 {
	q, ok := list[name]
	if !ok {
		return 0
	}

	return q.Value()
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}

	return b
}
