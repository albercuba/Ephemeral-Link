package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"ephemeral-link/internal/config"
	"ephemeral-link/internal/redisstore"
)

func main() {
	from := flag.String("from", redisstore.DefaultWorkspaceID, "source workspace ID")
	to := flag.String("to", "", "destination workspace ID")
	flag.Parse()
	if *to == "" {
		flag.Usage()
		os.Exit(2)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	store, err := redisstore.New(cfg.RedisURL)
	if err != nil {
		log.Error("Redis init failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	count, err := store.MigrateWorkspace(context.Background(), *from, *to)
	if err != nil {
		log.Error("workspace migration failed", "error", err, "updated", count)
		os.Exit(1)
	}
	log.Info("workspace migration complete", "from", *from, "to", *to, "updated", count)
}
