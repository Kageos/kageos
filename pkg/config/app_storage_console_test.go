package config

import "testing"

func TestConsoleAddressRequiresExplicitConfiguration(t *testing.T) {
	t.Setenv("MINIO_CONSOLE_URL", "")
	cfg := &AppStorageConfig{}
	cfg.Storage.Type = "minio"
	cfg.Storage.MinIO.Endpoint = "127.0.0.1:9000"
	if cfg.GetMinIOConsoleURL() != "" {
		t.Fatal("loopback is not evidence of development mode")
	}
	t.Setenv("MINIO_CONSOLE_URL", "https://ops.example.invalid")
	if cfg.GetMinIOConsoleURL() != "https://ops.example.invalid" {
		t.Fatal("explicit operations setting not preserved")
	}
}
