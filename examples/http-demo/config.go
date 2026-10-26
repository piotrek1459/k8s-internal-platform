package main

import (
	"fmt"
	"strconv"
	"strings"
)

// These defaults are embedded in each image through Go linker flags.
var (
	buildVersion   = "dev"
	buildUnhealthy = "false"
)

type config struct {
	port          int
	version       string
	readinessFail bool
}

func loadConfig(getenv func(string) string, version, unhealthy string) (config, error) {
	cfg := config{port: 8080, version: version}
	if strings.TrimSpace(version) == "" {
		return config{}, fmt.Errorf("embedded application version must not be empty")
	}
	if value := getenv("PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return config{}, fmt.Errorf("PORT must be an integer between 1 and 65535")
		}
		cfg.port = port
	}
	if value := getenv("READINESS_FAIL"); value != "" {
		unhealthy = value
	}
	value, err := strconv.ParseBool(unhealthy)
	if err != nil {
		return config{}, fmt.Errorf("readiness failure setting must be a boolean")
	}
	cfg.readinessFail = value
	return cfg, nil
}
