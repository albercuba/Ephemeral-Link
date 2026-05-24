package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Bundle struct { defaultLang string; allowed map[string]bool; messages map[string]map[string]string }
func Load(dir, def string, langs []string) (*Bundle, error) { b := &Bundle{defaultLang:def, allowed:map[string]bool{}, messages:map[string]map[string]string{}}; for _, lang := range langs { b.allowed[lang]=true; raw, err := os.ReadFile(filepath.Join(dir, lang+".json")); if err != nil { return nil, err }; m := map[string]string{}; if err := json.Unmarshal(raw, &m); err != nil { return nil, err }; b.messages[lang]=m }; return b, nil }
func (b *Bundle) Normalize(lang string) string { if b.allowed[lang] { return lang }; return b.defaultLang }
func (b *Bundle) T(lang, key string) string { lang = b.Normalize(lang); if v := b.messages[lang][key]; v != "" { return v }; if v := b.messages[b.defaultLang][key]; v != "" { return v }; return key }
func (b *Bundle) Allowed() []string { out := make([]string,0,len(b.allowed)); for k := range b.allowed { out=append(out,k) }; return out }
