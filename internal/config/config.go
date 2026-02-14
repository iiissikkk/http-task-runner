package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultHTTPAddr            = ":9091"
	defaultHTTPPort            = "9091"
	defaultDatabaseURL         = ""
	defaultHTTPExecutorTimeout = 10 * time.Second
	defaultHTTPReadTimeout     = 10 * time.Second
	defaultHTTPWriteTimeout    = 10 * time.Second
	defaultHTTPIdleTimeout     = 60 * time.Second
	defaultHTTPShutdownTimeout = 10 * time.Second
)

type Config struct {
	DatabaseURL         string
	HTTPAddr            string
	HTTPPort            string
	HTTPExecutorTimeout time.Duration
	HTTPReadTimeout     time.Duration
	HTTPWriteTimeout    time.Duration
	HTTPIdleTimeout     time.Duration
	HTTPShutdownTimeout time.Duration
}

func Load() (Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return Config{}, err
	}

	executorTimeout, err := getDurationEnv("HTTP_EXECUTOR_TIMEOUT", defaultHTTPExecutorTimeout)
	if err != nil {
		return Config{}, err
	}

	readTimeout, err := getDurationEnv("HTTP_SERVER_READ_TIMEOUT", defaultHTTPReadTimeout)
	if err != nil {
		return Config{}, err
	}

	writeTimeout, err := getDurationEnv("HTTP_SERVER_WRITE_TIMEOUT", defaultHTTPWriteTimeout)
	if err != nil {
		return Config{}, err
	}

	idleTimeout, err := getDurationEnv("HTTP_SERVER_IDLE_TIMEOUT", defaultHTTPIdleTimeout)
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := getDurationEnv("HTTP_SHUTDOWN_TIMEOUT", defaultHTTPShutdownTimeout)
	if err != nil {
		return Config{}, err
	}

	return Config{
		DatabaseURL:         getEnv("DATABASE_URL", defaultDatabaseURL),
		HTTPAddr:            getEnv("HTTP_ADDR", defaultHTTPAddr),
		HTTPPort:            getEnv("HTTP_PORT", defaultHTTPPort),
		HTTPExecutorTimeout: executorTimeout,
		HTTPReadTimeout:     readTimeout,
		HTTPWriteTimeout:    writeTimeout,
		HTTPIdleTimeout:     idleTimeout,
		HTTPShutdownTimeout: shutdownTimeout,
	}, nil
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}

	return duration, nil
}

func loadDotEnv(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to open %s: %w", path, err)
	}

	lines := strings.Split(string(content), "\n")
	for i, raw := range lines {
		lineNum := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid %s line %d: missing '='", path, lineNum)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("invalid %s line %d: empty key", path, lineNum)
		}

		value = strings.TrimSpace(value)

		// OS-level environment variables have higher priority than .env values.
		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to set %s from %s: %w", key, path, err)
		}
	}

	return nil
}
