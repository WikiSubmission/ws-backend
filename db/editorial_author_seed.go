package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Author auto-seeding. When a user is granted the article editorial module they
// need an author identity to be attributed on articles, so ReplaceEditorGrants
// ensures a 1:1 editorial_authors row exists, seeded from users.display_name.
// The row is a draft (never auto-published) and is fully editable afterwards by
// the user and by admins. The partial unique index from migration 025 keeps the
// link one-to-one; seeding is idempotent (a no-op once the row exists).

// ensureAuthorForUser creates the user's author profile inside tx if one does
// not already exist. It runs in the grant transaction so a failed seed rolls the
// whole grant change back.
func (db *DB) ensureAuthorForUser(ctx context.Context, tx pgx.Tx, userID int) error {
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM editorial_authors WHERE user_id = $1)`, userID,
	).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

	var displayName *string
	var email string
	if err := tx.QueryRow(ctx,
		`SELECT display_name, email FROM users WHERE id = $1`, userID,
	).Scan(&displayName, &email); err != nil {
		return err
	}

	firstName, lastName := splitDisplayName(displayName, email)

	base := slugifyName(firstName + " " + lastName)
	if base == "" {
		base = fmt.Sprintf("author-%d", userID)
	}
	slug, err := uniqueAuthorSlug(ctx, tx, base)
	if err != nil {
		return err
	}

	draft, err := json.Marshal(map[string]any{
		"first_name": firstName,
		"last_name":  lastName,
		"slug":       slug,
		"photo_url":  "",
		"bio":        "",
		"user_id":    userID,
	})
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO editorial_authors (slug, user_id, draft, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (user_id) WHERE user_id IS NOT NULL DO NOTHING`,
		slug, userID, draft, userID,
	); err != nil {
		return translateContentPgError(err)
	}
	return nil
}

// uniqueAuthorSlug returns base, or base-2, base-3, … — the first value not
// already taken in editorial_authors (slug is globally unique for that table).
func uniqueAuthorSlug(ctx context.Context, tx pgx.Tx, base string) (string, error) {
	slug := base
	for i := 2; ; i++ {
		var taken bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM editorial_authors WHERE slug = $1)`, slug,
		).Scan(&taken); err != nil {
			return "", err
		}
		if !taken {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

// splitDisplayName derives a (first, last) name pair. A single-token name keeps
// last empty (last_name is optional for authors). When display_name is blank the
// email local-part is used as the first name.
func splitDisplayName(displayName *string, email string) (string, string) {
	name := ""
	if displayName != nil {
		name = strings.TrimSpace(*displayName)
	}
	if name == "" {
		if at := strings.IndexByte(email, '@'); at > 0 {
			name = email[:at]
		} else {
			name = email
		}
	}
	fields := strings.Fields(name)
	switch len(fields) {
	case 0:
		return "Author", ""
	case 1:
		return fields[0], ""
	default:
		return fields[0], strings.Join(fields[1:], " ")
	}
}

// slugifyName lowercases, keeps [a-z0-9], collapses every other run into a single
// hyphen and trims leading/trailing hyphens — matching slugPattern
// (^[a-z0-9][a-z0-9-]*$). Returns "" when nothing usable remains.
func slugifyName(s string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
