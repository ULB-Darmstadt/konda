package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"git.rwth-aachen.de/dsma/publications/software/konda/analyzer"
	"git.rwth-aachen.de/dsma/publications/software/konda/app"
	"git.rwth-aachen.de/dsma/publications/software/konda/handler"
	"git.rwth-aachen.de/dsma/publications/software/konda/store"
	"github.com/alexedwards/scs/boltstore"
	"github.com/joho/godotenv"
	"go.etcd.io/bbolt"
)

func main() {
	if _, err := os.Stat(".env"); err == nil {
		err := godotenv.Load()
		if err != nil {
			slog.Warn("Could not load .env", "details", err.Error())
		} else {
			slog.Info(".env loaded successfully")
		}
	}

	// Initialize DBs
	db, err := bbolt.Open("db-sessions.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	store.InitSessionManager(boltstore.New(db))

	appStateStore, err := store.NewBoltStore("db-app.db")
	if err != nil {
		slog.Error("Failed to open app state store", "err", err)
		return
	}
	defer appStateStore.Close()
	err = analyzer.InitializeTaskManager(appStateStore)
	if err != nil {
		slog.Error("Failed to initialize the task manager", "details", err)
	}

	app := &app.App{
		Store: appStateStore,
	}

	handlerMux := handler.SetupRoutes(app)
	port, err := strconv.Atoi(getEnv("BACKEND_PORT", "5050"))
	if err != nil {
		slog.Error("Failed to convert BACKEND_PORT variable", "details", err.Error())
		return
	}
	server := &http.Server{
		Addr:         fmt.Sprintf("localhost:%d", port),
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 10 * time.Minute,
		Handler:      handlerMux,
	}

	// Graceful Server Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Error running the server", "details", err.Error())
		}
	}()
	slog.Info("Server started ...", "port", port)

	go store.CleanupExpiredSessions(app.Store)

	// Block until we receive a signal
	<-stop

	// Create a context with a timeout to allow for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server Shutdown Failed", "details", err)
	}
	defer store.DbClient.Driver.Close(store.DbClient.Ctx)

	// TODO: check how we clean up things
	// err = workspace.CleanUpAllWorkspaces()
	// if err != nil {
	// 	slog.Error("Error removing workspaces directory ", "details", err)
	// }

	slog.Info("Server gracefully stopped")
}

// Utilities

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
