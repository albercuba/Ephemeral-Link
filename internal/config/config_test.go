package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadRequiresMasterKey(t *testing.T) {
	t.Setenv("ENCRYPTION_MASTER_KEY", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "ENCRYPTION_MASTER_KEY is required") {
		t.Fatalf("Load() error = %v, want required master key error", err)
	}
}

func TestLoadAcceptsBase64PrefixedMasterKey(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	t.Setenv("ENCRYPTION_MASTER_KEY", "base64:"+key)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if string(cfg.EncryptionMasterKey) != "0123456789abcdef0123456789abcdef" {
		t.Fatal("unexpected decoded master key")
	}
}

func TestLoadRejectsInvalidMasterKeyLength(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("too-short"))
	t.Setenv("ENCRYPTION_MASTER_KEY", key)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "exactly 32 bytes") {
		t.Fatalf("Load() error = %v, want invalid length error", err)
	}
}

func TestLoadRejectsInvalidTTLConfiguration(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	t.Setenv("ENCRYPTION_MASTER_KEY", key)
	t.Setenv("DEFAULT_TTL_SECONDS", "120")
	t.Setenv("MAX_TTL_SECONDS", "60")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "invalid TTL") {
		t.Fatalf("Load() error = %v, want invalid TTL error", err)
	}
}

func TestLoadParsesTrustedProxies(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	t.Setenv("ENCRYPTION_MASTER_KEY", key)
	t.Setenv("TRUSTED_PROXIES", "10.0.0.1, 192.168.0.0/24, ")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0] != "10.0.0.1" || cfg.TrustedProxies[1] != "192.168.0.0/24" {
		t.Fatalf("TrustedProxies = %#v", cfg.TrustedProxies)
	}
}
