package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/wikiSubmission/ws-backend/db"
)

func main() {
	email := flag.String("email", "", "Email address of the user to promote to admin")
	flag.Parse()

	if *email == "" {
		fmt.Fprintln(os.Stderr, "Usage: seed-admin --email <user@example.com>")
		os.Exit(1)
	}

	ctx := context.Background()
	database, err := db.DbSetup()
	if err != nil {
		slog.Error("Database setup failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer database.Close()

	tag, err := database.Pool.Exec(ctx,
		`UPDATE users SET role = 'admin', updated_at = NOW() WHERE email = $1`,
		*email,
	)
	if err != nil {
		slog.Error("Failed to update user role", "email", *email, slog.Any("error", err))
		os.Exit(1)
	}

	if tag.RowsAffected() == 0 {
		fmt.Fprintf(os.Stderr, "No user found with email %q — make sure the user has logged in at least once.\n", *email)
		os.Exit(1)
	}

	fmt.Printf("User %q successfully promoted to admin.\n", *email)
}
