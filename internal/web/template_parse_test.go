package web

import (
	"html/template"
	"testing"
)

func TestTemplatesParse(t *testing.T) {
	_, err := template.New("").Funcs(template.FuncMap{
		"humanSize":   func(int64) string { return "" },
		"formatBytes": func(int64) string { return "" },
		"formatTime":  func(int64) string { return "" },
		"t":           func(string, Page) string { return "" },
	}).ParseGlob("../../web/templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
}
