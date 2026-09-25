package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLocaleKeySetsStayInSync(t *testing.T) {
	keys := make(map[string]map[string]json.RawMessage)
	for _, language := range []string{"en", "de"} {
		path := filepath.Join("..", "..", "locales", language+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var messages map[string]json.RawMessage
		if err := json.Unmarshal(data, &messages); err != nil {
			t.Fatalf("%s locale: %v", language, err)
		}
		keys[language] = messages
	}
	for key := range keys["en"] {
		if _, ok := keys["de"][key]; !ok {
			t.Errorf("German locale is missing key %q", key)
		}
	}
	for key := range keys["de"] {
		if _, ok := keys["en"][key]; !ok {
			t.Errorf("English locale is missing key %q", key)
		}
	}
}

func TestBundleFallsBackToDefaultLanguage(t *testing.T) {
	bundle, err := Load("../../locales", "en", []string{"en", "de"})
	if err != nil {
		t.Fatal(err)
	}
	if got := bundle.Normalize("de-DE"); got != "en" {
		t.Fatalf("Normalize(de-DE) = %q, want en when only exact languages are allowed", got)
	}
	if got := bundle.Match("de-DE,de;q=0.9"); got != "de" {
		t.Fatalf("Match(de-DE,...) = %q, want de", got)
	}
	if got := bundle.T("de", "missing_key"); got != "missing_key" {
		t.Fatalf("missing translation fallback = %q", got)
	}
}
