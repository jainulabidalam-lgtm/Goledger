package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Host         string
	Port         int
	DBPath       string
	TrustedProxy string
}

func Load() (*Config, error) {
	cfg := &Config{
		Host:   "127.0.0.1",
		Port:   8080,
		DBPath: "goledger.db",
	}

	if host := os.Getenv("HOST"); host != "" {
		if strings.ContainsAny(host, " \t\r\n\\/") {
			return nil, errors.New("invalid HOST configuration")
		}
		cfg.Host = host
	}

	if portStr := os.Getenv("PORT"); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return nil, errors.New("invalid PORT")
		}
		cfg.Port = port
	}

	if dbPath := os.Getenv("DB_PATH"); dbPath != "" {
		if strings.Contains(dbPath, "..") || strings.Contains(dbPath, "\x00") {
			return nil, errors.New("invalid DB_PATH configuration")
		}
		cfg.DBPath = dbPath
	}

	if tp := os.Getenv("TRUSTED_PROXY"); tp != "" {
		cfg.TrustedProxy = tp
	}

	return cfg, nil
}
