package storage

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Local struct{ root string }

func NewLocal(root string) (*Local, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Local{root: root}, nil
}
func (l *Local) Path(id string) string { return filepath.Join(l.root, id+".bin") }
func (l *Local) Write(id string, data []byte) (string, error) {
	p := l.Path(id)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return p, nil
}
func (l *Local) Read(path string) ([]byte, error)   { return os.ReadFile(path) }
func (l *Local) Open(path string) (*os.File, error) { return os.Open(path) }
func (l *Local) WriteStream(id string, write func(io.Writer) error) (string, error) {
	p := l.Path(id)
	tmp := p + ".tmp"
	file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	if err := write(file); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return p, nil
}
func (l *Local) Delete(path string) { _ = os.Remove(path) }
func (l *Local) CleanupOlderThan(age time.Duration) error {
	cutoff := time.Now().Add(-age)
	return filepath.WalkDir(l.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(p)
		}
		return nil
	})
}
func (l *Local) CleanupOrphans(active map[string]struct{}) error {
	return filepath.WalkDir(l.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Ext(p) != ".bin" {
			return nil
		}
		clean := filepath.Clean(p)
		if _, ok := active[clean]; !ok {
			_ = os.Remove(clean)
		}
		return nil
	})
}

var unsafeName = regexp.MustCompile(`[^a-zA-Z0-9._ -]+`)

func SanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = unsafeName.ReplaceAllString(name, "_")
	name = strings.Trim(name, ". ")
	if name == "" {
		return "download.bin"
	}
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}
