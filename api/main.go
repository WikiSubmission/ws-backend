// Package main is the entry point for the WikiSubmission Library API.
// It handles dependency injection, background worker orchestration, and server startup.
package main

import (
	"fmt"
	"log/slog"
	"os"
	_ "time/tzdata"

	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	"github.com/wikiSubmission/ws-backend/db"
)

func main() {
	// The offline desktop sidecar skips .env on purpose: a developer's .env
	// points at hosted services the desktop app must never reach. Its logs go
	// to stderr because stdout carries the readiness handshake.
	if len(os.Args) > 1 && os.Args[1] == "--desktop" {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
		os.Exit(runDesktop(os.Stdin, os.Stdout))
	}

	if err := godotenv.Load(); err != nil {
		fmt.Println("info: No .env file found, using system environment variables")
	}

	var handler slog.Handler

	if os.Getenv("APP_ENV") == "production" {
		handler = slog.NewJSONHandler(os.Stdout, nil)
	} else {
		handler = tint.NewHandler(os.Stdout, &tint.Options{
			Level:      slog.LevelDebug,
			TimeFormat: "15:04:05",
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	database, err := db.DbSetup()
	if err != nil {
		slog.Error("Database setup failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer database.Close()

	StartServer(database)
}
