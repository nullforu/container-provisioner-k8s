package stack

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"smctf/internal/config"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	sigsyaml "sigs.k8s.io/yaml"
)

type KubernetesClient struct {
	client            kubernetes.Interface
	schedulingTimeout time.Duration
}

type KubernetesClientAPI interface {
	CreatePodAndService(ctx context.Context, req ProvisionRequest) (ProvisionResult, error)
	DeletePodAndService(ctx context.Context, namespace, podID, serviceName string) error
	GetPodStatus(ctx context.Context, namespace, podID string) (Status, string, error)
	ListPods(ctx context.Context, namespace string) ([]string, error)
	ListServices(ctx context.Context, namespace string) ([]string, error)
	NodeExists(ctx context.Context, nodeID string) (bool, error)
	HasIngressNetworkPolicy(ctx context.Context, namespace string) (bool, error)
	GetNodePublicIP(ctx context.Context, nodeID string) (*string, error)
}

type ProvisionRequest struct {
	Namespace  string
	StackID    string
	PodSpecYML string
	TargetPort int
	NodePort   int
}

type ProvisionResult struct {
	PodID       string
	ServiceName string
	NodeID      string
	Status      Status
}

func NewKubernetesClient(cfg config.StackConfig) (*KubernetesClient, error) {
	restCfg, err := buildKubeConfig(cfg)
	if err != nil {
		return nil, err
	}
	restCfg.QPS = float32(cfg.K8sQPS)
	restCfg.Burst = cfg.K8sBurst

	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("new kubernetes client: %w", err)
	}

	return &KubernetesClient{
		client:            client,
		schedulingTimeout: cfg.SchedulingTimeout,
	}, nil
}

func buildKubeConfig(cfg config.StackConfig) (*rest.Config, error) {
	if cfg.KubeConfigPath != "" {
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: filepath.Clean(cfg.KubeConfigPath)}
		overrides := &clientcmd.ConfigOverrides{}
		if cfg.KubeContext != "" {
			overrides.CurrentContext = cfg.KubeContext
		}

		clientCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
		out, err := clientCfg.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("load kubeconfig: %w", err)
		}

		return out, nil
	}

	out, err := rest.InClusterConfig()
	if err == nil {
		return out, nil
	}

	clientCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: cfg.KubeContext})
	out, fallbackErr := clientCfg.ClientConfig()
	if fallbackErr != nil {
		return nil, fmt.Errorf("in-cluster and default kubeconfig failed: %v / %w", err, fallbackErr)
	}

	return out, nil
}

func (c *KubernetesClient) CreatePodAndService(ctx context.Context, req ProvisionRequest) (ProvisionResult, error) {
	if err := c.ensureNamespace(ctx, req.Namespace); err != nil {
		return ProvisionResult{}, err
	}

	var pod corev1.Pod
	if err := sigsyaml.Unmarshal([]byte(req.PodSpecYML), &pod); err != nil {
		return ProvisionResult{}, fmt.Errorf("decode pod spec: %w", err)
	}

	podName := req.StackID
	serviceName := "svc-" + req.StackID
	labels := make(map[string]string)
	if len(pod.Labels) > 0 {
		maps.Copy(labels, pod.Labels)
	}
	labels["app.kubernetes.io/name"] = "smctf-stack"
	labels["app.kubernetes.io/instance"] = req.StackID
	labels["smctf.io/stack-id"] = req.StackID

	pod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
	pod.ObjectMeta = metav1.ObjectMeta{
		Name:        podName,
		Namespace:   req.Namespace,
		Labels:      labels,
		Annotations: pod.Annotations,
	}

	createdPod, err := c.client.CoreV1().Pods(req.Namespace).Create(ctx, &pod, metav1.CreateOptions{})
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("create pod: %w", err)
	}

	protocol := corev1.ProtocolTCP
	for _, cn := range createdPod.Spec.Containers {
		for _, p := range cn.Ports {
			if int(p.ContainerPort) == req.TargetPort {
				if p.Protocol != "" {
					protocol = p.Protocol
				}
			}
		}
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: req.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: map[string]string{"smctf.io/stack-id": req.StackID},
			Ports: []corev1.ServicePort{
				{
					Name:       "challenge",
					Protocol:   protocol,
					Port:       int32(req.TargetPort),
					TargetPort: intstr.FromInt(req.TargetPort),
					NodePort:   int32(req.NodePort),
				},
			},
		},
	}

	_, err = c.client.CoreV1().Services(req.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		_ = c.client.CoreV1().Pods(req.Namespace).Delete(context.Background(), podName, metav1.DeleteOptions{GracePeriodSeconds: int64Ptr(0)})
		return ProvisionResult{}, fmt.Errorf("create service: %w", err)
	}

	waitCtx := ctx
	var cancel context.CancelFunc
	if c.schedulingTimeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, c.schedulingTimeout)
		defer cancel()
	}

	if err := c.waitUntilSchedulable(waitCtx, req.Namespace, podName); err != nil {
		_ = c.client.CoreV1().Services(req.Namespace).Delete(context.Background(), serviceName, metav1.DeleteOptions{})
		_ = c.client.CoreV1().Pods(req.Namespace).Delete(context.Background(), podName, metav1.DeleteOptions{GracePeriodSeconds: int64Ptr(0)})
		return ProvisionResult{}, err
	}

	createdPod, err = c.client.CoreV1().Pods(req.Namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("get pod after scheduling: %w", err)
	}

	return ProvisionResult{
		PodID:       podName,
		ServiceName: serviceName,
		NodeID:      createdPod.Spec.NodeName,
		Status:      mapPodPhaseToStatus(createdPod.Status.Phase),
	}, nil
}

func (c *KubernetesClient) DeletePodAndService(ctx context.Context, namespace, podID, serviceName string) error {
	if serviceName != "" {
		err := c.client.CoreV1().Services(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete service: %w", err)
		}
	}

	err := c.client.CoreV1().Pods(namespace).Delete(ctx, podID, metav1.DeleteOptions{GracePeriodSeconds: int64Ptr(0)})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete pod: %w", err)
	}

	return nil
}

func (c *KubernetesClient) GetPodStatus(ctx context.Context, namespace, podID string) (Status, string, error) {
	pod, err := c.client.CoreV1().Pods(namespace).Get(ctx, podID, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", "", ErrNotFound
		}

		return "", "", err
	}

	if pod.Status.Reason == "NodeLost" {
		return StatusNodeDeleted, pod.Spec.NodeName, nil
	}

	return mapPodPhaseToStatus(pod.Status.Phase), pod.Spec.NodeName, nil
}

func (c *KubernetesClient) ListPods(ctx context.Context, namespace string) ([]string, error) {
	podList, err := c.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	out := make([]string, 0, len(podList.Items))
	for _, item := range podList.Items {
		if item.Name == "" {
			continue
		}

		out = append(out, item.Name)
	}
	return out, nil
}

func (c *KubernetesClient) ListServices(ctx context.Context, namespace string) ([]string, error) {
	svcList, err := c.client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	out := make([]string, 0, len(svcList.Items))
	for _, item := range svcList.Items {
		if item.Name == "" {
			continue
		}

		out = append(out, item.Name)
	}
	return out, nil
}

func (c *KubernetesClient) NodeExists(ctx context.Context, nodeID string) (bool, error) {
	if nodeID == "" {
		return false, nil
	}

	_, err := c.client.CoreV1().Nodes().Get(ctx, nodeID, metav1.GetOptions{})
	if err == nil {
		return true, nil
	}

	if apierrors.IsNotFound(err) {
		return false, nil
	}

	return false, err
}

func (c *KubernetesClient) HasIngressNetworkPolicy(ctx context.Context, namespace string) (bool, error) {
	if namespace == "" {
		return false, nil
	}

	policies, err := c.client.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("list networkpolicies: %w", err)
	}

	for _, policy := range policies.Items {
		hasIngressType := slices.Contains(policy.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
		if !hasIngressType {
			continue
		}

		if len(policy.Spec.Ingress) == 0 {
			continue
		}

		return true, nil
	}

	return false, nil
}

func (c *KubernetesClient) GetNodePublicIP(ctx context.Context, nodeID string) (*string, error) {
	if nodeID == "" {
		return nil, nil
	}

	node, err := c.client.CoreV1().Nodes().Get(ctx, nodeID, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}

		return nil, err
	}

	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP && addr.Address != "" {
			return &addr.Address, nil
		}
	}

	return nil, nil
}

func (c *KubernetesClient) ensureNamespace(ctx context.Context, ns string) error {
	_, err := c.client.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err == nil {
		return nil
	}

	if !apierrors.IsNotFound(err) {
		return err
	}

	_, err = c.client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace: %w", err)
	}

	return nil
}

func mapPodPhaseToStatus(phase corev1.PodPhase) Status {
	switch phase {
	case corev1.PodRunning:
		return StatusRunning
	case corev1.PodFailed:
		return StatusFailed
	case corev1.PodSucceeded:
		return StatusStopped
	case corev1.PodPending:
		return StatusCreating
	default:
		return StatusCreating
	}
}

func (c *KubernetesClient) waitUntilSchedulable(ctx context.Context, namespace, podName string) error {
	err := wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		pod, err := c.client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, ErrNotFound
			}

			return false, err
		}

		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse && cond.Reason == corev1.PodReasonUnschedulable {
				return false, ErrClusterSaturated
			}
		}

		if pod.Spec.NodeName != "" {
			return true, nil
		}
		if pod.Status.Phase == corev1.PodFailed {
			return false, fmt.Errorf("pod failed before scheduling")
		}

		return false, nil
	})

	if err == nil {
		return nil
	}

	if errors.Is(err, ErrClusterSaturated) {
		return ErrClusterSaturated
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrClusterSaturated
	}

	return err
}
