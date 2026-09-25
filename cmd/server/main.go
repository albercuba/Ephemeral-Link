package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ephemeral-link/internal/cleanup"
	"ephemeral-link/internal/config"
	"ephemeral-link/internal/i18n"
	"ephemeral-link/internal/redisstore"
	"ephemeral-link/internal/storage"
	"ephemeral-link/internal/web"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	store, err := redisstore.New(cfg.RedisURL)
	if err != nil {
		log.Error("redis config failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	var files storage.Backend
	if cfg.StorageBackend == "s3" {
		files, err = storage.NewS3(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3SessionToken, cfg.S3Bucket, cfg.S3Prefix, cfg.S3Secure)
	} else if cfg.StorageBackend == "local" {
		files, err = storage.NewLocal(cfg.StoragePath)
	} else {
		err = fmt.Errorf("unsupported STORAGE_BACKEND %q", cfg.StorageBackend)
	}
	if err != nil {
		log.Error("storage init failed", "error", err)
		os.Exit(1)
	}
	bundle, err := i18n.Load("locales", cfg.DefaultLanguage, cfg.AllowedLanguages)
	if err != nil {
		log.Error("i18n init failed", "error", err)
		os.Exit(1)
	}
	app, err := web.New(cfg, store, files, bundle, log)
	if err != nil {
		log.Error("web init failed", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cleanup.Start(ctx, log, files, store, cfg.MaxTTL)
	srv := &http.Server{Addr: ":8080", Handler: app.Routes(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		log.Info("server started", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server failed", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
