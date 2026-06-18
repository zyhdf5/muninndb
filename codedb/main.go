package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/scrypster/muninndb/codedb/ui"
	webui "github.com/scrypster/muninndb/codedb/web"
	"github.com/scrypster/muninndb/internal/auth"
	"github.com/scrypster/muninndb/internal/cognitive"
	"github.com/scrypster/muninndb/internal/engine"
	"github.com/scrypster/muninndb/internal/engine/activation"
	"github.com/scrypster/muninndb/internal/engine/trigger"
	"github.com/scrypster/muninndb/internal/index/fts"
	hnswpkg "github.com/scrypster/muninndb/internal/index/hnsw"
	"github.com/scrypster/muninndb/internal/logging"
	"github.com/scrypster/muninndb/internal/plugin"
	"github.com/scrypster/muninndb/internal/storage"
	"github.com/scrypster/muninndb/internal/transport/rest"
)

func main() {
	dataDir := flag.String("data", defaultDataDir(), "data directory")
	uiAddr := flag.String("ui-addr", ":8476", "UI server listen address")
	restAddr := flag.String("rest-addr", ":8475", "REST API listen address (internal, used by UI proxy)")
	dev := flag.Bool("dev", false, "serve web assets from filesystem instead of embedded")
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn, error")
	flag.Parse()

	// Configure logging.
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(*logLevel)); err != nil {
		fmt.Fprintf(os.Stderr, "invalid --log-level %q\n", *logLevel)
		os.Exit(1)
	}
	ring := logging.NewRingBuffer(1000, nil)
	baseHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(logging.NewRingHandler(baseHandler, ring)))

	// Resolve web FS.
	var webFS fs.FS = webui.FS
	if *dev {
		webDir := filepath.Join(filepath.Dir(os.Args[0]), "web")
		if _, err := os.Stat(webDir); err != nil {
			webDir = "codedb/web"
		}
		webFS = os.DirFS(webDir)
		slog.Info("dev mode: serving web assets from filesystem", "dir", webDir)
	}

	// Open Pebble DB.
	dbPath := filepath.Join(*dataDir, "pebble")
	if err := os.MkdirAll(dbPath, 0700); err != nil {
		slog.Error("create data dir", "err", err)
		os.Exit(1)
	}
	db, err := storage.OpenPebble(dbPath, storage.DefaultOptions())
	if err != nil {
		slog.Error("open pebble", "err", err)
		os.Exit(1)
	}

	// Auth.
	authStore := auth.NewStore(db)
	secretPath := filepath.Join(*dataDir, "auth_secret")
	sessionSecret, err := auth.Bootstrap(authStore, secretPath)
	if err != nil {
		slog.Error("auth bootstrap failed", "err", err)
		os.Exit(1)
	}

	// Storage layer.
	store := storage.NewPebbleStore(db, storage.PebbleStoreConfig{CacheSize: 10000})

	// Indexes.
	ftsIndex := fts.New(db)
	hnswRegistry := hnswpkg.NewRegistry(db)

	// Noop embedder (no ML model needed for the codedb standalone).
	embedder := activation.NewNoopEmbedder()

	// Activation engine + trigger system.
	actEngine := activation.New(store, activation.NewFTSAdapter(ftsIndex), activation.NewHNSWAdapter(hnswRegistry), embedder)
	trigSystem := trigger.New(store, trigger.NewFTSAdapter(ftsIndex), trigger.NewHNSWAdapter(hnswRegistry), embedder)

	// Signal handling context.
	ctx, cancel := context.WithCancel(context.Background())

	// Cognitive workers.
	hebbianWorkerImpl := cognitive.NewHebbianWorker(cognitive.NewHebbianStoreAdapter(store))
	contradictWorkerImpl := cognitive.NewContradictWorker(cognitive.NewContradictStoreAdapter(store))
	confidenceWorkerImpl := cognitive.NewConfidenceWorker(cognitive.NewConfidenceStoreAdapter(store))

	transitionWorkerImpl := cognitive.NewTransitionWorker(ctx, store.TransitionCache())
	actEngine.SetTransitionStore(store.TransitionCache())

	// Build engine.
	eng := engine.NewEngine(engine.EngineConfig{
		Store:            store,
		AuthStore:        authStore,
		FTSIndex:         ftsIndex,
		ActivationEngine: actEngine,
		TriggerSystem:    trigSystem,
		HebbianWorker:    hebbianWorkerImpl,
		ContradictWorker: contradictWorkerImpl.Worker,
		ConfidenceWorker: confidenceWorkerImpl.Worker,
		Embedder:         embedder,
		HNSWRegistry:     hnswRegistry,
	})
	eng.SetTransitionWorker(transitionWorkerImpl)

	// REST wrapper + server (internal — its Handler() is proxied by the UI server).
	restWrapper := rest.NewEngineWrapper(eng, hnswRegistry)
	pluginRegistry := plugin.NewRegistry()
	embedInfo := rest.EmbedInfo{Provider: "none"}
	enrichInfo := rest.EnrichInfo{}
	restServer := rest.NewServer(*restAddr, restWrapper, authStore, sessionSecret, nil, embedInfo, enrichInfo, pluginRegistry, *dataDir, nil)

	// UI server — proxies /api/* to REST handler, serves codedb web assets.
	uiSrv, err := ui.NewServer(webFS, restWrapper, restServer.Handler(), authStore, sessionSecret, ring, nil, nil)
	if err != nil {
		slog.Error("create ui server", "err", err)
		os.Exit(1)
	}

	// Wire log broadcast.
	ring.SetOnAdd(func(e logging.LogEntry) {
		data, _ := json.Marshal(map[string]any{
			"type":  "log_entry",
			"level": e.Level,
			"time":  e.Time.Format(time.RFC3339),
			"msg":   e.Msg,
			"attrs": e.Attrs,
		})
		uiSrv.Broadcast(data)
	})

	// Start trigger system.
	trigSystem.Start(ctx)

	// Start cognitive workers.
	go contradictWorkerImpl.Worker.Run(ctx)
	go confidenceWorkerImpl.Worker.Run(ctx)

	// Start REST server (internal).
	go func() {
		slog.Info("REST server starting", "addr", *restAddr)
		if err := restServer.Serve(ctx); err != nil {
			slog.Error("REST server error", "err", err)
		}
	}()

	// Start UI server.
	if err := uiSrv.Start(ctx, *uiAddr); err != nil {
		slog.Error("start ui server", "err", err)
		os.Exit(1)
	}
	slog.Info("codedb UI server listening", "addr", *uiAddr)
	slog.Info("codedb started — open your browser at http://127.0.0.1" + *uiAddr)

	// Signal handling.
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("shutdown signal received")
		cancel()
		<-sigCh
		slog.Error("second signal — forcing exit")
		os.Exit(1)
	}()

	// Wait for context cancellation.
	<-ctx.Done()

	// Graceful shutdown.
	slog.Info("shutting down")
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutCancel()
		restServer.Shutdown(shutCtx)
		if err := uiSrv.Stop(shutCtx); err != nil {
			slog.Error("ui server shutdown error", "err", err)
		}
		eng.Stop()
		hebbianWorkerImpl.Stop()
		transitionWorkerImpl.Stop()
		if err := store.Close(); err != nil {
			slog.Error("store close error", "err", err)
		}
	}()
	select {
	case <-shutdownDone:
		slog.Info("shutdown complete")
	case <-time.After(20 * time.Second):
		slog.Error("shutdown timed out; forcing exit")
		os.Exit(1)
	}
}

// defaultDataDir returns a sensible default data directory.
func defaultDataDir() string {
	if d := os.Getenv("CODEDB_DATA"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".codedb-data"
	}
	return filepath.Join(home, ".codedb", "data")
}
