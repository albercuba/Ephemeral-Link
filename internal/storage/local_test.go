package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := map[string]string{
		"../../../secret.txt": "secret.txt",
		" report?.csv ":       "report_.csv",
		"...":                 "download.bin",
		"":                    "download.bin",
	}
	for input, want := range tests {
		if got := SanitizeFilename(input); got != want {
			t.Fatalf("SanitizeFilename(%q) = %q, want %q", input, got, want)
		}
	}
	if got := SanitizeFilename(strings.Repeat("a", 140) + ".txt"); len(got) != 120 {
		t.Fatalf("long filename length = %d, want 120", len(got))
	}
}

func TestCleanupOrphansDeletesOnlyInactiveBinFiles(t *testing.T) {
	st, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	activePath, err := st.Write("active", []byte("active"))
	if err != nil {
		t.Fatal(err)
	}
	orphanPath, err := st.Write("orphan", []byte("orphan"))
	if err != nil {
		t.Fatal(err)
	}
	nonBin := filepath.Join(filepath.Dir(activePath), "brand-logo")
	if err := os.WriteFile(nonBin, []byte("logo"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := st.CleanupOrphans(map[string]struct{}{filepath.Clean(activePath): {}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(activePath); err != nil {
		t.Fatalf("active file was removed: %v", err)
	}
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Fatalf("orphan file still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(nonBin); err != nil {
		t.Fatalf("non-bin file was removed: %v", err)
	}
}
