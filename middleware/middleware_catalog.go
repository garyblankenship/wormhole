package middleware

// MiddlewareInfo describes available middleware
type MiddlewareInfo struct {
	Name       string
	Purpose    string
	Example    string
	ConfigType string
}

// AvailableMiddleware returns information about all available middleware
func AvailableMiddleware() []MiddlewareInfo {
	return []MiddlewareInfo{
		{
			Name:       "NewTypedCacheMiddleware",
			Purpose:    "Response caching with TTL support",
			Example:    "middleware.NewTypedCacheMiddleware(middleware.CacheConfig{Cache: cache, TTL: 5*time.Minute})",
			ConfigType: "CacheConfig",
		},
		{
			Name:       "NewTypedCircuitBreakerMiddleware",
			Purpose:    "Circuit breaking for failing providers",
			Example:    "middleware.NewTypedCircuitBreakerMiddleware(5, 30*time.Second)",
			ConfigType: "threshold int, timeout time.Duration",
		},
		{
			Name:       "NewTypedRateLimitMiddleware",
			Purpose:    "Request rate limiting",
			Example:    "middleware.NewTypedRateLimitMiddleware(100)",
			ConfigType: "requestsPerSecond int",
		},
		{
			Name:       "NewTypedLoggingMiddleware",
			Purpose:    "Request/response logging",
			Example:    "middleware.NewTypedLoggingMiddleware(middleware.DefaultLoggingConfig(logger))",
			ConfigType: "logger types.Logger",
		},
		{
			Name:       "NewTypedMetricsMiddleware",
			Purpose:    "Request metrics collection",
			Example:    "middleware.NewTypedMetricsMiddleware(metrics)",
			ConfigType: "metrics *TypedMetrics",
		},
		{
			Name:       "NewTypedTimeoutMiddleware",
			Purpose:    "Request timeout enforcement",
			Example:    "middleware.NewTypedTimeoutMiddleware(30*time.Second)",
			ConfigType: "timeout time.Duration",
		},
	}
}
