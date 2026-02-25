// I started to hate DynamoDB while working on this...
// I miss PostgreSQL and SQL queries so much.

// 코드 유지보수하는 후배들아 미안하다.. -_-;;; 어차피 AI 돌리는같긴 하지만..
// - 김준영

package stack

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	ddbPK = "pk"
	ddbSK = "sk"

	ddbGSIAllPK   = "gsi1pk"
	ddbGSIAllSK   = "gsi1sk"
	ddbGSIAllName = "gsi1"
	ddbAllPKValue = "STACKS"
)

type DynamoRepository struct {
	client         *dynamodb.Client
	table          string
	consistentRead bool
	portLockTTL    time.Duration
	rand           *rand.Rand
	randMu         sync.Mutex
}

func NewDynamoRepository(client *dynamodb.Client, table string, consistentRead bool, portLockTTL time.Duration) *DynamoRepository {
	return &DynamoRepository{
		client:         client,
		table:          table,
		consistentRead: consistentRead,
		portLockTTL:    portLockTTL,
		rand:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (r *DynamoRepository) Create(ctx context.Context, st Stack) error {
	now := nowRFC3339()
	base := stackToItem(st)

	byID := copyItem(base)
	byID[ddbPK] = avS(stackMetaPK(st.StackID))
	byID[ddbSK] = avS("META")
	byID["item_type"] = avS("stack_by_id")

	if len(st.Ports) == 0 {
		return ErrNoAvailableNodePort
	}

	items := []ddtypes.TransactWriteItem{
		{Put: &ddtypes.Put{TableName: &r.table, Item: byID, ConditionExpression: strPtr("attribute_not_exists(pk) AND attribute_not_exists(sk)")}},
	}

	for _, p := range st.Ports {
		keyPort := map[string]ddtypes.AttributeValue{ddbPK: avS("PORTS"), ddbSK: avS(portSK(p.NodePort))}
		items = append(items, ddtypes.TransactWriteItem{Update: &ddtypes.Update{
			TableName:                 &r.table,
			Key:                       keyPort,
			UpdateExpression:          strPtr("SET stack_id = :sid, updated_at = :now"),
			ConditionExpression:       strPtr("attribute_exists(pk) AND attribute_exists(sk) AND (attribute_not_exists(stack_id) OR stack_id = :empty)"),
			ExpressionAttributeValues: map[string]ddtypes.AttributeValue{":sid": avS(st.StackID), ":now": avS(now), ":empty": avS("")},
		}})
	}

	_, err := r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: items,
	})

	if err != nil {
		return mapDynamoTxError(err)
	}

	return nil
}

func (r *DynamoRepository) Get(ctx context.Context, stackID string) (Stack, bool, error) {
	resp, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      &r.table,
		ConsistentRead: boolPtr(r.consistentRead),
		Key: map[string]ddtypes.AttributeValue{
			ddbPK: avS(stackMetaPK(stackID)),
			ddbSK: avS("META"),
		},
	})

	if err != nil {
		return Stack{}, false, err
	}

	if len(resp.Item) == 0 {
		return Stack{}, false, nil
	}

	st, err := stackFromItem(resp.Item)
	if err != nil {
		return Stack{}, false, err
	}

	return st, true, nil
}

func (r *DynamoRepository) Delete(ctx context.Context, stackID string) (Stack, bool, error) {
	st, ok, err := r.Get(ctx, stackID)
	if err != nil {
		return Stack{}, false, err
	}

	if !ok {
		return Stack{}, false, nil
	}

	items := []ddtypes.TransactWriteItem{
		{Delete: &ddtypes.Delete{
			TableName:           &r.table,
			Key:                 map[string]ddtypes.AttributeValue{ddbPK: avS(stackMetaPK(st.StackID)), ddbSK: avS("META")},
			ConditionExpression: strPtr("attribute_exists(pk) AND attribute_exists(sk)"),
		}},
	}
	for _, p := range st.Ports {
		items = append(items, ddtypes.TransactWriteItem{Delete: &ddtypes.Delete{
			TableName: &r.table,
			Key:       map[string]ddtypes.AttributeValue{ddbPK: avS("PORTS"), ddbSK: avS(portSK(p.NodePort))},
		}})
	}

	_, err = r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: items,
	})

	if err != nil {
		if txConditionFailedAt(err, 0) {
			return Stack{}, false, nil
		}
		return Stack{}, false, err
	}

	return st, true, nil
}

func (r *DynamoRepository) ListAll(ctx context.Context) ([]Stack, error) {
	out := make([]Stack, 0)
	var startKey map[string]ddtypes.AttributeValue
	for {
		resp, err := r.client.Query(ctx, &dynamodb.QueryInput{
			TableName:              &r.table,
			IndexName:              strPtr(ddbGSIAllName),
			KeyConditionExpression: strPtr(ddbGSIAllPK + " = :pk"),
			ExpressionAttributeValues: map[string]ddtypes.AttributeValue{
				":pk": avS(ddbAllPKValue),
			},
			ExclusiveStartKey: startKey,
		})

		if err != nil {
			return nil, err
		}

		for _, item := range resp.Items {
			st, err := stackFromItem(item)
			if err != nil {
				return nil, err
			}
			out = append(out, st)
		}

		if len(resp.LastEvaluatedKey) == 0 {
			break
		}

		startKey = resp.LastEvaluatedKey
	}

	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (r *DynamoRepository) ReserveNodePort(ctx context.Context, min, max int) (int, error) {
	total := max - min + 1
	if total <= 0 {
		return 0, ErrNoAvailableNodePort
	}

	start := r.randomInt(total)
	now := nowRFC3339()
	nowUnix := time.Now().UTC().Unix()
	staleBefore := nowUnix - int64(r.portLockTTL.Seconds())
	for i := range total {
		port := min + ((start + i) % total)
		_, err := r.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: &r.table,
			Item: map[string]ddtypes.AttributeValue{
				ddbPK:        avS("PORTS"),
				ddbSK:        avS(portSK(port)),
				"item_type":  avS("port_lock"),
				"port":       avN(strconv.Itoa(port)),
				"created_at": avS(now),
				"locked_at":  avN(strconv.FormatInt(nowUnix, 10)),
				"stack_id":   avS(""),
			},
			ConditionExpression: strPtr("attribute_not_exists(pk) AND attribute_not_exists(sk)"),
		})

		if err == nil {
			return port, nil
		}

		var condErr *ddtypes.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			_, reclaimErr := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName: &r.table,
				Key: map[string]ddtypes.AttributeValue{
					ddbPK: avS("PORTS"),
					ddbSK: avS(portSK(port)),
				},
				UpdateExpression:    strPtr("SET created_at = :now, locked_at = :locked_at"),
				ConditionExpression: strPtr("(attribute_not_exists(stack_id) OR stack_id = :empty) AND (attribute_not_exists(locked_at) OR locked_at < :stale_before)"),
				ExpressionAttributeValues: map[string]ddtypes.AttributeValue{
					":now":          avS(now),
					":locked_at":    avN(strconv.FormatInt(nowUnix, 10)),
					":empty":        avS(""),
					":stale_before": avN(strconv.FormatInt(staleBefore, 10)),
				},
			})

			if reclaimErr == nil {
				return port, nil
			}

			continue
		}

		return 0, err
	}

	return 0, ErrNoAvailableNodePort
}

func (r *DynamoRepository) ReleaseNodePort(ctx context.Context, port int) error {
	_, err := r.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: &r.table,
		Key: map[string]ddtypes.AttributeValue{
			ddbPK: avS("PORTS"),
			ddbSK: avS(portSK(port)),
		},
		ConditionExpression:       strPtr("attribute_not_exists(stack_id) OR stack_id = :empty"),
		ExpressionAttributeValues: map[string]ddtypes.AttributeValue{":empty": avS("")},
	})

	var condErr *ddtypes.ConditionalCheckFailedException
	if errors.As(err, &condErr) {
		return nil
	}

	return err
}

func (r *DynamoRepository) UsedNodePortCount(ctx context.Context) (int, error) {
	total := 0
	var startKey map[string]ddtypes.AttributeValue

	for {
		resp, err := r.client.Query(ctx, &dynamodb.QueryInput{
			TableName:              &r.table,
			KeyConditionExpression: strPtr("pk = :pk"),
			ExpressionAttributeValues: map[string]ddtypes.AttributeValue{
				":pk": avS("PORTS"),
			},
			Select:            ddtypes.SelectCount,
			ExclusiveStartKey: startKey,
		})

		if err != nil {
			return 0, err
		}

		total += int(resp.Count)
		if len(resp.LastEvaluatedKey) == 0 {
			break
		}

		startKey = resp.LastEvaluatedKey
	}

	return total, nil
}

func (r *DynamoRepository) UpdateStatus(ctx context.Context, stackID string, status Status, nodeID string) error {
	now := nowRFC3339()
	values := map[string]ddtypes.AttributeValue{
		":status": avS(string(status)),
		":node":   avS(nodeID),
		":now":    avS(now),
	}

	_, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.table,
		Key:                       map[string]ddtypes.AttributeValue{ddbPK: avS(stackMetaPK(stackID)), ddbSK: avS("META")},
		UpdateExpression:          strPtr("SET #status = :status, node_id = :node, updated_at = :now"),
		ConditionExpression:       strPtr("attribute_exists(pk) AND attribute_exists(sk)"),
		ExpressionAttributeNames:  map[string]string{"#status": "status"},
		ExpressionAttributeValues: values,
	})

	if err != nil {
		var condErr *ddtypes.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return ErrNotFound
		}

		return err
	}

	return nil
}

func mapDynamoTxError(err error) error {
	var txErr *ddtypes.TransactionCanceledException
	if !errors.As(err, &txErr) {
		return err
	}

	for idx, reason := range txErr.CancellationReasons {
		if reason.Code == nil || *reason.Code != "ConditionalCheckFailed" {
			continue
		}
		switch idx {
		case 0:
			return err
		default:
			return ErrNoAvailableNodePort
		}
	}

	return err
}

func txConditionFailedAt(err error, index int) bool {
	var txErr *ddtypes.TransactionCanceledException
	if !errors.As(err, &txErr) {
		return false
	}

	if index < 0 || index >= len(txErr.CancellationReasons) {
		return false
	}

	reason := txErr.CancellationReasons[index]

	return reason.Code != nil && *reason.Code == "ConditionalCheckFailed"
}

func stackToItem(st Stack) map[string]ddtypes.AttributeValue {
	item := map[string]ddtypes.AttributeValue{
		"stack_id":               avS(st.StackID),
		ddbGSIAllPK:              avS(ddbAllPKValue),
		ddbGSIAllSK:              avS(st.CreatedAt.UTC().Format(time.RFC3339Nano)),
		"pod_id":                 avS(st.PodID),
		"namespace":              avS(st.Namespace),
		"node_id":                avS(st.NodeID),
		"pod_spec":               avS(st.PodSpecYAML),
		"target_ports":           portSpecsToAttr(st.TargetPorts),
		"ports":                  portMappingsToAttr(st.Ports),
		"service_name":           avS(st.ServiceName),
		"status":                 avS(string(st.Status)),
		"ttl_expires_at":         avS(st.TTLExpiresAt.UTC().Format(time.RFC3339Nano)),
		"created_at":             avS(st.CreatedAt.UTC().Format(time.RFC3339Nano)),
		"updated_at":             avS(st.UpdatedAt.UTC().Format(time.RFC3339Nano)),
		"requested_cpu_milli":    avN(strconv.FormatInt(st.RequestedMilli, 10)),
		"requested_memory_bytes": avN(strconv.FormatInt(st.RequestedBytes, 10)),
	}

	if st.NodePublicIP != nil {
		item["node_public_ip"] = avS(*st.NodePublicIP)
	}

	return item
}

func stackFromItem(item map[string]ddtypes.AttributeValue) (Stack, error) {
	stackID, err := attrString(item, "stack_id")
	if err != nil {
		return Stack{}, err
	}

	podID, _ := attrString(item, "pod_id")
	namespace, _ := attrString(item, "namespace")
	nodeID, _ := attrString(item, "node_id")
	nodePublicIP := attrStringOptional(item, "node_public_ip")
	podSpec, _ := attrString(item, "pod_spec")
	targetPorts, _ := attrPortSpecs(item, "target_ports")
	portMappings, _ := attrPortMappings(item, "ports")
	serviceName, _ := attrString(item, "service_name")
	statusStr, _ := attrString(item, "status")
	ttlAt, err := attrTime(item, "ttl_expires_at")
	if err != nil {
		return Stack{}, err
	}

	createdAt, err := attrTime(item, "created_at")
	if err != nil {
		return Stack{}, err
	}

	updatedAt, err := attrTime(item, "updated_at")
	if err != nil {
		return Stack{}, err
	}

	cpuMilli, _ := attrInt64(item, "requested_cpu_milli")
	memBytes, _ := attrInt64(item, "requested_memory_bytes")

	return Stack{
		StackID:        stackID,
		PodID:          podID,
		Namespace:      namespace,
		NodeID:         nodeID,
		NodePublicIP:   nodePublicIP,
		PodSpecYAML:    podSpec,
		TargetPorts:    targetPorts,
		Ports:          portMappings,
		ServiceName:    serviceName,
		Status:         Status(statusStr),
		TTLExpiresAt:   ttlAt,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		RequestedMilli: cpuMilli,
		RequestedBytes: memBytes,
	}, nil
}

func attrString(item map[string]ddtypes.AttributeValue, key string) (string, error) {
	v, ok := item[key]
	if !ok {
		return "", fmt.Errorf("missing attribute %s", key)
	}

	s, ok := v.(*ddtypes.AttributeValueMemberS)
	if !ok {
		return "", fmt.Errorf("attribute %s is not string", key)
	}

	return s.Value, nil
}

func attrStringOptional(item map[string]ddtypes.AttributeValue, key string) *string {
	v, ok := item[key]
	if !ok {
		return nil
	}

	s, ok := v.(*ddtypes.AttributeValueMemberS)
	if !ok || s.Value == "" {
		return nil
	}

	return &s.Value
}

func attrInt(item map[string]ddtypes.AttributeValue, key string) (int, error) {
	n, err := attrInt64(item, key)
	if err != nil {
		return 0, err
	}

	return int(n), nil
}

func attrInt64(item map[string]ddtypes.AttributeValue, key string) (int64, error) {
	v, ok := item[key]
	if !ok {
		return 0, fmt.Errorf("missing attribute %s", key)
	}

	n, ok := v.(*ddtypes.AttributeValueMemberN)
	if !ok {
		return 0, fmt.Errorf("attribute %s is not number", key)
	}

	out, err := strconv.ParseInt(n.Value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("attribute %s parse failed", key)
	}

	return out, nil
}

func attrTime(item map[string]ddtypes.AttributeValue, key string) (time.Time, error) {
	s, err := attrString(item, key)
	if err != nil {
		return time.Time{}, err
	}

	return time.Parse(time.RFC3339Nano, s)
}

func attrPortSpecs(item map[string]ddtypes.AttributeValue, key string) ([]PortSpec, error) {
	v, ok := item[key]
	if !ok {
		return nil, nil
	}

	list, ok := v.(*ddtypes.AttributeValueMemberL)
	if !ok {
		return nil, fmt.Errorf("attribute %s is not list", key)
	}

	out := make([]PortSpec, 0, len(list.Value))
	for _, entry := range list.Value {
		m, ok := entry.(*ddtypes.AttributeValueMemberM)
		if !ok {
			return nil, fmt.Errorf("attribute %s entry is not map", key)
		}

		port, err := attrInt(m.Value, "container_port")
		if err != nil {
			return nil, err
		}

		proto, _ := attrString(m.Value, "protocol")
		out = append(out, PortSpec{ContainerPort: port, Protocol: proto})
	}

	return out, nil
}

func attrPortMappings(item map[string]ddtypes.AttributeValue, key string) ([]PortMapping, error) {
	v, ok := item[key]
	if !ok {
		return nil, nil
	}

	list, ok := v.(*ddtypes.AttributeValueMemberL)
	if !ok {
		return nil, fmt.Errorf("attribute %s is not list", key)
	}

	out := make([]PortMapping, 0, len(list.Value))
	for _, entry := range list.Value {
		m, ok := entry.(*ddtypes.AttributeValueMemberM)
		if !ok {
			return nil, fmt.Errorf("attribute %s entry is not map", key)
		}

		port, err := attrInt(m.Value, "container_port")
		if err != nil {
			return nil, err
		}

		nodePort, err := attrInt(m.Value, "node_port")
		if err != nil {
			return nil, err
		}

		proto, _ := attrString(m.Value, "protocol")
		out = append(out, PortMapping{ContainerPort: port, Protocol: proto, NodePort: nodePort})
	}

	return out, nil
}

func portSpecsToAttr(ports []PortSpec) ddtypes.AttributeValue {
	list := make([]ddtypes.AttributeValue, 0, len(ports))
	for _, p := range ports {
		list = append(list, &ddtypes.AttributeValueMemberM{Value: map[string]ddtypes.AttributeValue{
			"container_port": avN(strconv.Itoa(p.ContainerPort)),
			"protocol":       avS(p.Protocol),
		}})
	}

	return &ddtypes.AttributeValueMemberL{Value: list}
}

func portMappingsToAttr(ports []PortMapping) ddtypes.AttributeValue {
	list := make([]ddtypes.AttributeValue, 0, len(ports))
	for _, p := range ports {
		list = append(list, &ddtypes.AttributeValueMemberM{Value: map[string]ddtypes.AttributeValue{
			"container_port": avN(strconv.Itoa(p.ContainerPort)),
			"protocol":       avS(p.Protocol),
			"node_port":      avN(strconv.Itoa(p.NodePort)),
		}})
	}

	return &ddtypes.AttributeValueMemberL{Value: list}
}

func copyItem(src map[string]ddtypes.AttributeValue) map[string]ddtypes.AttributeValue {
	out := make(map[string]ddtypes.AttributeValue, len(src))
	maps.Copy(out, src)

	return out
}

// helper functions

func stackMetaPK(stackID string) string { return "STACK#" + stackID }
func stackSK(stackID string) string     { return "STACK#" + stackID }
func portSK(port int) string            { return "PORT#" + strconv.Itoa(port) }

func avS(v string) ddtypes.AttributeValue { return &ddtypes.AttributeValueMemberS{Value: v} }
func avN(v string) ddtypes.AttributeValue { return &ddtypes.AttributeValueMemberN{Value: v} }
func (r *DynamoRepository) randomInt(limit int) int {
	r.randMu.Lock()
	defer r.randMu.Unlock()

	return r.rand.Intn(limit)
}
