package utils

import "time"

func Int64Ptr(v int64) *int64        { return &v }
func TimePtr(t time.Time) *time.Time { return &t }
