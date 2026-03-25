package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultAddr                = ":8080"
	defaultDataDir             = "data"
	defaultMaxUploadMB         = 100
	defaultOutputRetentionDays = 30
)

type Config struct {
	Addr                string
	DataDir             string
	DatabaseURL         string
	MaxUploadBytes      int64
	AutoMigrate         bool
	OutputRetentionDays int
}

func Load() Config {
	dataDir := firstNonEmpty(os.Getenv("TESTRR_DATA_DIR"), defaultDataDir)
	addr := firstNonEmpty(os.Getenv("TESTRR_ADDR"), defaultAddr)
	maxUploadMB := parseInt64(firstNonEmpty(os.Getenv("TESTRR_MAX_UPLOAD_MB"), strconv.FormatInt(defaultMaxUploadMB, 10)), defaultMaxUploadMB)
	autoMigrate := parseBool(firstNonEmpty(os.Getenv("TESTRR_AUTO_MIGRATE"), "true"))
	outputRetentionDays := parseInt(firstNonEmpty(os.Getenv("TESTRR_OUTPUT_RETENTION_DAYS"), strconv.Itoa(defaultOutputRetentionDays)), defaultOutputRetentionDays)
	databaseURL := os.Getenv("TESTRR_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = filepath.Join(dataDir, "testrr.sqlite")
	}

	return Config{
		Addr:                addr,
		DataDir:             dataDir,
		DatabaseURL:         databaseURL,
		MaxUploadBytes:      maxUploadMB * 1024 * 1024,
		AutoMigrate:         autoMigrate,
		OutputRetentionDays: outputRetentionDays,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func parseInt64(value string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
