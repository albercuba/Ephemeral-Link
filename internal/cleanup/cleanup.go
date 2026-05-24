package cleanup

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"ephemeral-link/internal/redisstore"
	"ephemeral-link/internal/storage"
)

func Start(ctx context.Context, log *slog.Logger, st *storage.Local, store *redisstore.Store, maxTTL time.Duration) {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := st.CleanupOlderThan(maxTTL + time.Hour); err != nil {
					log.Warn("storage cleanup failed", "error", err)
				}
				if store == nil {
					continue
				}
				active, err := store.ActiveStorageObjectPaths(ctx)
				if err != nil {
					log.Warn("active storage lookup failed", "error", err)
					continue
				}
				cleanActive := make(map[string]struct{}, len(active))
				for p := range active {
					cleanActive[filepath.Clean(p)] = struct{}{}
				}
				if err := st.CleanupOrphans(cleanActive); err != nil {
					log.Warn("orphan storage cleanup failed", "error", err)
				}
			}
		}
	}()
}
