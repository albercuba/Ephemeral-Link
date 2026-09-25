package web

import (
	"strings"
	"testing"
)

func TestRenderLegalMarkdownStripsFrontMatterAndEscapesHTML(t *testing.T) {
	rendered, err := renderLegalMarkdown("---\ntitle: Test\n---\n# Heading\n\n<script>alert(1)</script>")
	if err != nil {
		t.Fatal(err)
	}
	output := string(rendered)
	if !strings.Contains(output, "<h1>Heading</h1>") {
		t.Fatalf("heading missing from rendered markdown: %s", output)
	}
	if strings.Contains(output, "<script>") || !strings.Contains(output, "raw HTML omitted") {
		t.Fatalf("unsafe HTML was not removed: %s", output)
	}
	if strings.Contains(output, "title: Test") {
		t.Fatalf("front matter was rendered: %s", output)
	}
}

func TestDefaultLegalDocumentsAreAvailable(t *testing.T) {
	documents := defaultLegalDocuments()
	if !strings.Contains(documents.Privacy, "# Privacy Policy") {
		t.Fatal("default privacy document is missing")
	}
	if !strings.Contains(documents.Terms, "# Terms of Service") {
		t.Fatal("default terms document is missing")
	}
}
