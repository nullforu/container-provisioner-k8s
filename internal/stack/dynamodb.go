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

func (r *DynamoRepository) Create(ctx context.Context, st Stack, constraints CreateConstraints) error {
	now := nowRFC3339()
	base := stackToItem(st)

	byID := copyItem(base)
	byID[ddbPK] = avS(stackMetaPK(st.StackID))
	byID[ddbSK] = avS("META")
	byID["item_type"] = avS("stack_by_id")

	keyGlobal := map[string]ddtypes.AttributeValue{ddbPK: avS("GLOBAL"), ddbSK: avS("RESOURCES")}
	keyPort := map[string]ddtypes.AttributeValue{ddbPK: avS("PORTS"), ddbSK: avS(portSK(st.NodePort))}
	cpuBefore := constraints.MaxReservedCPUMilli - st.RequestedMilli
	memBefore := constraints.MaxReservedMemoryBytes - st.RequestedBytes
	if cpuBefore <= 0 || memBefore <= 0 {
		return ErrClusterSaturated
	}

	_, err := r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []ddtypes.TransactWriteItem{
			{Put: &ddtypes.Put{TableName: &r.table, Item: byID, ConditionExpression: strPtr("attribute_not_exists(pk) AND attribute_not_exists(sk)")}},
			{Update: &ddtypes.Update{
				TableName:           &r.table,
				Key:                 keyGlobal,
				UpdateExpression:    strPtr("SET reserved_cpu_milli = if_not_exists(reserved_cpu_milli, :zero) + :cpu, reserved_memory_bytes = if_not_exists(reserved_memory_bytes, :zero) + :mem, updated_at = :now"),
				ConditionExpression: strPtr("(attribute_not_exists(reserved_cpu_milli) OR reserved_cpu_milli < :cpu_before) AND (attribute_not_exists(reserved_memory_bytes) OR reserved_memory_bytes < :mem_before)"),
				ExpressionAttributeValues: map[string]ddtypes.AttributeValue{
					":zero":       avN("0"),
					":cpu":        avN(strconv.FormatInt(st.RequestedMilli, 10)),
					":mem":        avN(strconv.FormatInt(st.RequestedBytes, 10)),
					":cpu_before": avN(strconv.FormatInt(cpuBefore, 10)),
					":mem_before": avN(strconv.FormatInt(memBefore, 10)),
					":now":        avS(now),
				},
			}},
			{Update: &ddtypes.Update{
				TableName:                 &r.table,
				Key:                       keyPort,
				UpdateExpression:          strPtr("SET stack_id = :sid, updated_at = :now"),
				ConditionExpression:       strPtr("attribute_exists(pk) AND attribute_exists(sk) AND (attribute_not_exists(stack_id) OR stack_id = :empty)"),
				ExpressionAttributeValues: map[string]ddtypes.AttributeValue{":sid": avS(st.StackID), ":now": avS(now), ":empty": avS("")},
			}},
		},
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

	now := nowRFC3339()
	negCPU := -st.RequestedMilli
	negMem := -st.RequestedBytes

	_, err = r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []ddtypes.TransactWriteItem{
			{Delete: &ddtypes.Delete{
				TableName:           &r.table,
				Key:                 map[string]ddtypes.AttributeValue{ddbPK: avS(stackMetaPK(st.StackID)), ddbSK: avS("META")},
				ConditionExpression: strPtr("attribute_exists(pk) AND attribute_exists(sk)"),
			}},
			{Delete: &ddtypes.Delete{TableName: &r.table, Key: map[string]ddtypes.AttributeValue{ddbPK: avS("PORTS"), ddbSK: avS(portSK(st.NodePort))}}},
			{Update: &ddtypes.Update{
				TableName:                 &r.table,
				Key:                       map[string]ddtypes.AttributeValue{ddbPK: avS("GLOBAL"), ddbSK: avS("RESOURCES")},
				UpdateExpression:          strPtr("SET reserved_cpu_milli = if_not_exists(reserved_cpu_milli, :zero) + :cpu, reserved_memory_bytes = if_not_exists(reserved_memory_bytes, :zero) + :mem, updated_at = :now"),
				ConditionExpression:       strPtr("attribute_exists(reserved_cpu_milli) AND attribute_exists(reserved_memory_bytes) AND reserved_cpu_milli >= :cpu_abs AND reserved_memory_bytes >= :mem_abs"),
				ExpressionAttributeValues: map[string]ddtypes.AttributeValue{":zero": avN("0"), ":cpu": avN(strconv.FormatInt(negCPU, 10)), ":mem": avN(strconv.FormatInt(negMem, 10)), ":cpu_abs": avN(strconv.FormatInt(st.RequestedMilli, 10)), ":mem_abs": avN(strconv.FormatInt(st.RequestedBytes, 10)), ":now": avS(now)},
			}},
		},
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
		case 1:
			return ErrClusterSaturated
		case 2:
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
		"target_port":            avN(strconv.Itoa(st.TargetPort)),
		"node_port":              avN(strconv.Itoa(st.NodePort)),
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
	targetPort, _ := attrInt(item, "target_port")
	nodePort, _ := attrInt(item, "node_port")
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
		TargetPort:     targetPort,
		NodePort:       nodePort,
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
