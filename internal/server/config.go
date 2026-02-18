package server

import (
	"os"
	"strconv"
	"time"
)

// Config holds server configuration from environment variables.
type Config struct {
	Port                string
	GRPCPort            string
	APIKey              string
	AllowInsecureNoAuth bool

	// HTTP server timeouts
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	// Shutdown drain timeout
	ShutdownTimeout time.Duration

	// Retention policies
	RetentionCompleted time.Duration
	RetentionCancelled time.Duration
	RetentionDiscarded time.Duration

	// Persistence
	PersistPath string
}

// LoadConfig reads configuration from environment variables with defaults.
func LoadConfig() Config {
	return Config{
		Port:                getEnv("OJS_PORT", "8080"),
		GRPCPort:            getEnv("OJS_GRPC_PORT", "9090"),
		APIKey:              getEnv("OJS_API_KEY", ""),
		AllowInsecureNoAuth: getEnvBool("OJS_ALLOW_INSECURE_NO_AUTH", true),

		ReadTimeout:  getDurationEnv("OJS_READ_TIMEOUT", 30*time.Second),
		WriteTimeout: getDurationEnv("OJS_WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:  getDurationEnv("OJS_IDLE_TIMEOUT", 120*time.Second),

		ShutdownTimeout: getDurationEnv("OJS_SHUTDOWN_TIMEOUT", 30*time.Second),

		RetentionCompleted: getDurationEnv("OJS_RETENTION_COMPLETED", 1*time.Hour),
		RetentionCancelled: getDurationEnv("OJS_RETENTION_CANCELLED", 24*time.Hour),
		RetentionDiscarded: getDurationEnv("OJS_RETENTION_DISCARDED", 24*time.Hour),

		PersistPath: getEnv("OJS_PERSIST", ""),
	}
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

func getDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
