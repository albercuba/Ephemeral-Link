package storage

import (
	"io"
	"time"
)

// Backend is the streaming encrypted-payload storage boundary. Paths returned
// by WriteStream are opaque backend references and must not be exposed to users.
type Backend interface {
	Write(id string, data []byte) (string, error)
	WriteStream(id string, write func(io.Writer) error) (string, error)
	Open(path string) (io.ReadCloser, error)
	Delete(path string)
	CleanupOlderThan(age time.Duration) error
	CleanupOrphans(active map[string]struct{}) error
}
