package main

import "testing"

func TestConfigDefaultsAndReadinessOverride(t *testing.T) {
	lookup := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}
	cfg, err := loadConfig(lookup(nil), "v3", "true")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.port != 8080 || cfg.version != "v3" || !cfg.readinessFail {
		t.Fatalf("unexpected image defaults: %+v", cfg)
	}
	cfg, err = loadConfig(lookup(map[string]string{"PORT": "9090", "READINESS_FAIL": "false"}), "v3", "true")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.port != 9090 || cfg.version != "v3" || cfg.readinessFail {
		t.Fatalf("runtime configuration did not override operational settings: %+v", cfg)
	}
}

func TestConfigRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		version   string
		unhealthy string
	}{
		{name: "zero port", env: map[string]string{"PORT": "0"}, version: "v1", unhealthy: "false"},
		{name: "negative port", env: map[string]string{"PORT": "-1"}, version: "v1", unhealthy: "false"},
		{name: "port out of range", env: map[string]string{"PORT": "65536"}, version: "v1", unhealthy: "false"},
		{name: "non-numeric port", env: map[string]string{"PORT": "http"}, version: "v1", unhealthy: "false"},
		{name: "invalid readiness override", env: map[string]string{"READINESS_FAIL": "sometimes"}, version: "v1", unhealthy: "false"},
		{name: "invalid image readiness", version: "v3", unhealthy: "invalid"},
		{name: "missing image version", version: " ", unhealthy: "false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadConfig(func(key string) string { return tt.env[key] }, tt.version, tt.unhealthy)
			if err == nil {
				t.Fatal("invalid configuration was accepted")
			}
		})
	}
}
