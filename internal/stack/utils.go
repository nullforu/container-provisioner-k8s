package stack

import "time"

func boolPtr(v bool) *bool    { return &v }
func int64Ptr(v int64) *int64 { return &v }
func strPtr(v string) *string { return &v }

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339Nano) }
