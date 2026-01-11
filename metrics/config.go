package metrics

type Config struct {
	APIBaseURL               string
	OAuthToken               string
	ListenPort               string
	CacheInvalidationSeconds int
	MetricsPath              string
}
