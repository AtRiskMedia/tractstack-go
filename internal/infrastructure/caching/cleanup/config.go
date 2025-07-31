package cleanup

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// Config holds cleanup worker configuration, sourced from the central config package.
type Config struct {
	CleanupInterval   time.Duration
	VerboseReporting  bool
	ContentCacheTTL   time.Duration
	SessionCacheTTL   time.Duration
	AnalyticsCacheTTL time.Duration
	FragmentCacheTTL  time.Duration
}

// NewConfig creates a new cleanup configuration by reading values
// from the already-initialized variables in the centralized /pkg/config package.
func NewConfig() *Config {
	return &Config{
		CleanupInterval:   config.RepositoryCleanupInterval,
		VerboseReporting:  config.RepositoryCleanupVerbose,
		ContentCacheTTL:   config.ContentCacheTTL,
		SessionCacheTTL:   config.UserStateTTL,
		AnalyticsCacheTTL: config.AnalyticsBinTTL,
		FragmentCacheTTL:  config.HTMLChunkTTL,
	}
}
