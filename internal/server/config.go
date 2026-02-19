package server

import (
	"time"

	commonconfig "github.com/openjobspec/ojs-go-backend-common/config"
)

// Config holds server configuration from environment variables.
type Config struct {
	commonconfig.BaseConfig

	// Retention policies
	RetentionCompleted time.Duration
	RetentionCancelled time.Duration
	RetentionDiscarded time.Duration

	// Persistence
	PersistPath string
}

// LoadConfig reads configuration from environment variables with defaults.
func LoadConfig() Config {
	base := commonconfig.LoadBaseConfig()
	// Lite backend defaults to allowing insecure access for development
	base.AllowInsecureNoAuth = commonconfig.GetEnvBool("OJS_ALLOW_INSECURE_NO_AUTH", true)

	return Config{
		BaseConfig: base,

		RetentionCompleted: commonconfig.GetDurationEnv("OJS_RETENTION_COMPLETED", 1*time.Hour),
		RetentionCancelled: commonconfig.GetDurationEnv("OJS_RETENTION_CANCELLED", 24*time.Hour),
		RetentionDiscarded: commonconfig.GetDurationEnv("OJS_RETENTION_DISCARDED", 24*time.Hour),

		PersistPath: commonconfig.GetEnv("OJS_PERSIST", ""),
	}
}
