package activitylog

const (
	DefaultPageSize = 50
	MaxPageSize     = 100
	// Match the default gRPC client receive limit before accumulating a response.
	MaxResponseBytes = 4 << 20
)

func PageSize(limit int32) int32 {
	if limit <= 0 {
		return DefaultPageSize
	}
	if limit > MaxPageSize {
		return MaxPageSize
	}
	return limit
}
