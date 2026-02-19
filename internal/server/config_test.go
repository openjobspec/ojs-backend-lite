package server

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	// Clear any OJS env vars
	for _, key := range []string{"OJS_PORT", "OJS_GRPC_PORT", "OJS_API_KEY", "OJS_ALLOW_INSECURE_NO_AUTH", "OJS_PERSIST"} {
		os.Unsetenv(key)
	}

	cfg := LoadConfig()

	if cfg.Port != "8080" {
		t.Errorf("expected port 8080, got %s", cfg.Port)
	}
	if cfg.GRPCPort != "9090" {
		t.Errorf("expected gRPC port 9090, got %s", cfg.GRPCPort)
	}
	if cfg.APIKey != "" {
		t.Error("expected empty API key by default")
	}
	if !cfg.AllowInsecureNoAuth {
		t.Error("expected AllowInsecureNoAuth to be true by default")
	}
	if cfg.ReadTimeout != 30*time.Second {
		t.Errorf("expected ReadTimeout 30s, got %v", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 30*time.Second {
		t.Errorf("expected WriteTimeout 30s, got %v", cfg.WriteTimeout)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("expected ShutdownTimeout 30s, got %v", cfg.ShutdownTimeout)
	}
	if cfg.RetentionCompleted != 1*time.Hour {
		t.Errorf("expected RetentionCompleted 1h, got %v", cfg.RetentionCompleted)
	}
	if cfg.PersistPath != "" {
		t.Errorf("expected empty PersistPath, got %s", cfg.PersistPath)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	os.Setenv("OJS_PORT", "3000")
	os.Setenv("OJS_GRPC_PORT", "50051")
	os.Setenv("OJS_API_KEY", "secret-key")
	os.Setenv("OJS_ALLOW_INSECURE_NO_AUTH", "false")
	os.Setenv("OJS_PERSIST", "/tmp/ojs.db")
	os.Setenv("OJS_READ_TIMEOUT", "10s")
	defer func() {
		os.Unsetenv("OJS_PORT")
		os.Unsetenv("OJS_GRPC_PORT")
		os.Unsetenv("OJS_API_KEY")
		os.Unsetenv("OJS_ALLOW_INSECURE_NO_AUTH")
		os.Unsetenv("OJS_PERSIST")
		os.Unsetenv("OJS_READ_TIMEOUT")
	}()

	cfg := LoadConfig()

	if cfg.Port != "3000" {
		t.Errorf("expected port 3000, got %s", cfg.Port)
	}
	if cfg.GRPCPort != "50051" {
		t.Errorf("expected gRPC port 50051, got %s", cfg.GRPCPort)
	}
	if cfg.APIKey != "secret-key" {
		t.Errorf("expected API key secret-key, got %s", cfg.APIKey)
	}
	if cfg.AllowInsecureNoAuth {
		t.Error("expected AllowInsecureNoAuth to be false")
	}
	if cfg.PersistPath != "/tmp/ojs.db" {
		t.Errorf("expected PersistPath /tmp/ojs.db, got %s", cfg.PersistPath)
	}
	if cfg.ReadTimeout != 10*time.Second {
		t.Errorf("expected ReadTimeout 10s, got %v", cfg.ReadTimeout)
	}
}

func TestLoadConfigInvalidDuration(t *testing.T) {
	os.Setenv("OJS_READ_TIMEOUT", "not-a-duration")
	defer os.Unsetenv("OJS_READ_TIMEOUT")

	cfg := LoadConfig()

	// Should fall back to default
	if cfg.ReadTimeout != 30*time.Second {
		t.Errorf("expected default ReadTimeout 30s for invalid input, got %v", cfg.ReadTimeout)
	}
}

func TestLoadConfigInvalidBool(t *testing.T) {
	os.Setenv("OJS_ALLOW_INSECURE_NO_AUTH", "not-a-bool")
	defer os.Unsetenv("OJS_ALLOW_INSECURE_NO_AUTH")

	cfg := LoadConfig()

	// Should fall back to default (true)
	if !cfg.AllowInsecureNoAuth {
		t.Error("expected AllowInsecureNoAuth to fall back to true for invalid input")
	}
}
