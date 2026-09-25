package web

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"ephemeral-link/internal/redisstore"
)

//go:embed legal/privacy.md legal/terms.md
var defaultLegalFiles embed.FS

func defaultLegalDocuments() redisstore.LegalDocuments {
	privacy, _ := defaultLegalFiles.ReadFile("legal/privacy.md")
	terms, _ := defaultLegalFiles.ReadFile("legal/terms.md")
	return redisstore.LegalDocuments{Privacy: string(privacy), Terms: string(terms)}
}

func (a *App) legalDocuments(ctx context.Context) (redisstore.LegalDocuments, error) {
	// This small adapter keeps the default fallback in one place while allowing
	// Redis to remain the source of truth after an administrator saves edits.
	documents, err := a.store.GetLegalDocuments(ctx)
	if err != nil {
		return redisstore.LegalDocuments{}, err
	}
	defaults := defaultLegalDocuments()
	if strings.TrimSpace(documents.Privacy) == "" {
		documents.Privacy = defaults.Privacy
	}
	if strings.TrimSpace(documents.Terms) == "" {
		documents.Terms = defaults.Terms
	}
	return documents, nil
}

func renderLegalMarkdown(source string) (template.HTML, error) {
	source = stripLegalFrontMatter(source)
	var rendered bytes.Buffer
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	if err := md.Convert([]byte(source), &rendered); err != nil {
		return "", fmt.Errorf("render legal document: %w", err)
	}
	return template.HTML(rendered.String()), nil
}

func stripLegalFrontMatter(source string) string {
	if !strings.HasPrefix(source, "---\n") {
		return source
	}
	if end := strings.Index(source[4:], "\n---"); end >= 0 {
		return strings.TrimLeft(source[4+end+len("\n---"):], "\n")
	}
	return source
}
