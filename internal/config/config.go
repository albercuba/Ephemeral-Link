package config

import (
	"encoding/base64"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppBaseURL          string
	RedisURL            string
	StoragePath         string
	StorageBackend      string
	S3Endpoint          string
	S3AccessKey         string
	S3SecretKey         string
	S3SessionToken      string
	S3Bucket            string
	S3Prefix            string
	S3Secure            bool
	MaxTextSecretSize   int64
	MaxFileSize         int64
	DefaultTTL          time.Duration
	MaxTTL              time.Duration
	EncryptionMasterKey []byte
	RateLimitPerMinute  int
	AllowedLanguages    []string
	DefaultLanguage     string
	SecureCookies       bool
	TrustedProxies      []string
	CustomDomains       []string
	DefaultWorkspaceID  string
}

func Load() (Config, error) {
	cfg := Config{
		AppBaseURL:         env("APP_BASE_URL", "http://localhost:8080"),
		RedisURL:           env("REDIS_URL", "redis://localhost:6379/0"),
		StoragePath:        env("STORAGE_PATH", "./data/storage"),
		StorageBackend:     strings.ToLower(env("STORAGE_BACKEND", "local")),
		S3Endpoint:         env("S3_ENDPOINT", ""),
		S3AccessKey:        env("S3_ACCESS_KEY", ""),
		S3SecretKey:        env("S3_SECRET_KEY", ""),
		S3SessionToken:     env("S3_SESSION_TOKEN", ""),
		S3Bucket:           env("S3_BUCKET", ""),
		S3Prefix:           env("S3_PREFIX", "ephemeral-link"),
		S3Secure:           strings.EqualFold(env("S3_SECURE", "true"), "true"),
		MaxTextSecretSize:  envInt64("MAX_TEXT_SECRET_SIZE", 64*1024),
		MaxFileSize:        envInt64("MAX_FILE_SIZE", 10*1024*1024),
		DefaultTTL:         time.Duration(envInt64("DEFAULT_TTL_SECONDS", 3600)) * time.Second,
		MaxTTL:             time.Duration(envInt64("MAX_TTL_SECONDS", 30*24*3600)) * time.Second,
		RateLimitPerMinute: int(envInt64("RATE_LIMIT_PER_MINUTE", 60)),
		AllowedLanguages:   split(env("ALLOWED_LANGUAGES", "en,de")),
		DefaultLanguage:    env("DEFAULT_LANGUAGE", "en"),
		SecureCookies:      strings.EqualFold(env("SECURE_COOKIES", "false"), "true"),
		TrustedProxies:     split(env("TRUSTED_PROXIES", "")),
		CustomDomains:      split(env("CUSTOM_DOMAINS", "")),
		DefaultWorkspaceID: env("DEFAULT_WORKSPACE_ID", "default"),
	}
	key, err := loadMasterKey(env("ENCRYPTION_MASTER_KEY", ""))
	if err != nil {
		return cfg, err
	}
	cfg.EncryptionMasterKey = key
	if cfg.DefaultTTL <= 0 || cfg.MaxTTL <= 0 || cfg.DefaultTTL > cfg.MaxTTL {
		return cfg, errors.New("invalid TTL configuration")
	}
	return cfg, nil
}

func loadMasterKey(raw string) ([]byte, error) {
	if raw == "" {
		return nil, errors.New("ENCRYPTION_MASTER_KEY is required; use 32 random bytes base64-encoded")
	}
	if strings.HasPrefix(raw, "base64:") {
		raw = strings.TrimPrefix(raw, "base64:")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("ENCRYPTION_MASTER_KEY must be base64 encoded")
	}
	if len(key) != 32 {
		return nil, errors.New("ENCRYPTION_MASTER_KEY must decode to exactly 32 bytes")
	}
	return key, nil
}

func env(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
func envInt64(k string, d int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return d
	}
	return n
}
func split(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
