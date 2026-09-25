package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"ephemeral-link/internal/config"
	"ephemeral-link/internal/redisstore"
	"ephemeral-link/internal/storage"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	if cfg.StorageBackend != "s3" {
		log.Error("set STORAGE_BACKEND=s3 before running migration")
		os.Exit(1)
	}
	local, err := storage.NewLocal(cfg.StoragePath)
	if err != nil {
		log.Error("local storage init failed", "error", err)
		os.Exit(1)
	}
	s3, err := storage.NewS3(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3SessionToken, cfg.S3Bucket, cfg.S3Prefix, cfg.S3Secure)
	if err != nil {
		log.Error("S3 storage init failed", "error", err)
		os.Exit(1)
	}
	store, err := redisstore.New(cfg.RedisURL)
	if err != nil {
		log.Error("Redis init failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	items, err := store.ListAvailableItems(context.Background())
	if err != nil {
		log.Error("active item listing failed", "error", err)
		os.Exit(1)
	}
	for _, item := range items {
		if item.StorageObjectPath == "" {
			continue
		}
		reader, err := local.Open(item.StorageObjectPath)
		if err != nil {
			log.Error("source object open failed", "item", item.ID, "error", err)
			os.Exit(1)
		}
		path, writeErr := s3.WriteStream(item.ID, func(writer io.Writer) error {
			_, err := io.Copy(writer, reader)
			return err
		})
		_ = reader.Close()
		if writeErr != nil {
			log.Error("S3 object write failed", "item", item.ID, "error", writeErr)
			os.Exit(1)
		}
		if err := store.UpdateStorageObjectPath(context.Background(), item.ID, path); err != nil {
			log.Error("Redis path update failed; source retained for recovery", "item", item.ID, "error", err)
			os.Exit(1)
		}
		fmt.Printf("migrated %s -> %s\n", item.ID, path)
	}
	log.Info("storage migration complete", "items", len(items), "source_deleted", false)
}
