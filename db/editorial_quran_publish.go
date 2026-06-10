package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Quran publish-request state machine + the approve-time sync of a chapter's
// drafts into the version-keyed editorial published tables. The live reader
// corpus is never touched here (see migration 019 / docs).

// Publish-request lifecycle errors. Handlers map these to 404 / 409.
var (
	ErrPublishRequestNotFound   = errors.New("publish request not found")
	ErrPublishRequestPending    = errors.New("a publish request is already pending for this chapter")
	ErrPublishRequestNotPending = errors.New("publish request is not pending")
)

// Changelog action labels.
const (
	ActionVerseDraftSaved   = "verse_draft_saved"
	ActionChapterDraftSaved = "chapter_draft_saved"
	ActionPublishRequested  = "publish_requested"
	ActionPublishApproved   = "publish_approved"
	ActionPublishRejected   = "publish_rejected"
	ActionPublishCancelled  = "publish_cancelled"
	ActionPublished         = "published"
)

// PublishRequestRow mirrors a row of public.quran_publish_requests.
type PublishRequestRow struct {
	ID            int
	VersionID     int
	ChapterNumber int
	Status        string
	Note          *string
	RequestedBy   int
	RequestedAt   time.Time
	ReviewedBy    *int
	ReviewedAt    *time.Time
	ReviewNote    *string
}

// ChangelogEntry is an append-only editorial event to record.
type ChangelogEntry struct {
	VersionID        int
	ChapterNumber    *int
	VerseNumber      *int
	Action           string
	ActorID          int
	PublishRequestID *int
	Detail           map[string]any
}

// ChangelogRow mirrors a row of public.quran_editorial_changelog.
type ChangelogRow struct {
	ID               int
	VersionID        int
	ChapterNumber    *int
	VerseNumber      *int
	Action           string
	ActorID          int
	PublishRequestID *int
	Detail           json.RawMessage
	CreatedAt        time.Time
}

const publishRequestColumns = `id, version_id, chapter_number, status::text, note,
	requested_by, requested_at, reviewed_by, reviewed_at, review_note`

func scanPublishRequest(row pgx.Row) (*PublishRequestRow, error) {
	var r PublishRequestRow
	err := row.Scan(&r.ID, &r.VersionID, &r.ChapterNumber, &r.Status, &r.Note,
		&r.RequestedBy, &r.RequestedAt, &r.ReviewedBy, &r.ReviewedAt, &r.ReviewNote)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// FetchPublishRequestByID returns one request, or (nil, ErrPublishRequestNotFound).
func (db *DB) FetchPublishRequestByID(ctx context.Context, id int) (*PublishRequestRow, error) {
	r, err := scanPublishRequest(db.Pool.QueryRow(ctx,
		`SELECT `+publishRequestColumns+` FROM quran_publish_requests WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPublishRequestNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("fetch_publish_request: %w", err)
	}
	return r, nil
}

// FetchPendingPublishRequest returns the open request for a version+chapter, or
// (nil, nil) if none is open.
func (db *DB) FetchPendingPublishRequest(ctx context.Context, versionID, chapterNumber int) (*PublishRequestRow, error) {
	r, err := scanPublishRequest(db.Pool.QueryRow(ctx,
		`SELECT `+publishRequestColumns+` FROM quran_publish_requests
		 WHERE version_id = $1 AND chapter_number = $2 AND status = 'pending'`, versionID, chapterNumber))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetch_pending_publish_request: %w", err)
	}
	return r, nil
}

// FetchPendingPublishRequestsByVersion returns all open requests for a version.
func (db *DB) FetchPendingPublishRequestsByVersion(ctx context.Context, versionID int) ([]PublishRequestRow, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+publishRequestColumns+` FROM quran_publish_requests
		 WHERE version_id = $1 AND status = 'pending'`, versionID)
	if err != nil {
		return nil, fmt.Errorf("fetch_pending_publish_requests_by_version: %w", err)
	}
	defer rows.Close()
	var out []PublishRequestRow
	for rows.Next() {
		r, err := scanPublishRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("fetch_pending_publish_requests_by_version: scan: %w", err)
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// CreatePublishRequest opens a pending request for a version+chapter and records
// a changelog entry, atomically. Returns ErrPublishRequestPending if one is
// already open for that chapter.
func (db *DB) CreatePublishRequest(ctx context.Context, versionID, chapterNumber, requestedBy int, note *string) (*PublishRequestRow, error) {
	defer observe("create_publish_request", time.Now())
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	r, err := scanPublishRequest(tx.QueryRow(ctx,
		`INSERT INTO quran_publish_requests (version_id, chapter_number, requested_by, note)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+publishRequestColumns, versionID, chapterNumber, requestedBy, note))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "quran_publish_requests_one_pending" {
			return nil, ErrPublishRequestPending
		}
		return nil, fmt.Errorf("create_publish_request: %w", err)
	}
	ch := chapterNumber
	if err := insertChangelog(ctx, tx, ChangelogEntry{
		VersionID: versionID, ChapterNumber: &ch, Action: ActionPublishRequested,
		ActorID: requestedBy, PublishRequestID: &r.ID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("create_publish_request: commit: %w", err)
	}
	return r, nil
}

// ListPublishRequests returns requests visible to the caller. Admins see all;
// other callers see requests they submitted plus requests for versions they may
// approve. status and versionID are optional filters.
func (db *DB) ListPublishRequests(ctx context.Context, viewerID int, isAdmin bool, approverVersionIDs []int, status *string, versionID *int) ([]PublishRequestRow, error) {
	defer observe("list_publish_requests", time.Now())
	var where []string
	args := []any{}
	add := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if !isAdmin {
		// requested_by = viewer OR version_id in approvable set.
		vis := fmt.Sprintf("requested_by = %s", add(viewerID))
		if len(approverVersionIDs) > 0 {
			vis = fmt.Sprintf("(%s OR version_id = ANY(%s))", vis, add(approverVersionIDs))
		}
		where = append(where, vis)
	}
	if status != nil {
		// add() parameterizes the value; the ::cast is a fixed token.
		where = append(where, fmt.Sprintf("status = %s::quran_publish_status", add(*status)))
	}
	if versionID != nil {
		where = append(where, "version_id = "+add(*versionID))
	}

	query := `SELECT ` + publishRequestColumns + ` FROM quran_publish_requests`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY requested_at DESC"

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list_publish_requests: %w", err)
	}
	defer rows.Close()
	var out []PublishRequestRow
	for rows.Next() {
		r, err := scanPublishRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("list_publish_requests: scan: %w", err)
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// ApprovePublishRequest transitions a pending request to approved and syncs that
// chapter's drafts into the editorial published tables, all in one transaction.
func (db *DB) ApprovePublishRequest(ctx context.Context, requestID, reviewerID int, note *string) (*PublishRequestRow, error) {
	defer observe("approve_publish_request", time.Now())
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	req, err := lockPendingRequest(ctx, tx, requestID)
	if err != nil {
		return nil, err
	}

	chapterID, err := chapterIDByNumber(ctx, tx, req.ChapterNumber)
	if err != nil {
		return nil, err
	}

	verses, err := syncVersePublished(ctx, tx, req.VersionID, chapterID, req.ID)
	if err != nil {
		return nil, err
	}
	if err := syncChapterPublished(ctx, tx, req.VersionID, chapterID, req.ID); err != nil {
		return nil, err
	}

	updated, err := scanPublishRequest(tx.QueryRow(ctx,
		`UPDATE quran_publish_requests
		 SET status='approved', reviewed_by=$2, reviewed_at=now(), review_note=$3, updated_at=now()
		 WHERE id=$1 RETURNING `+publishRequestColumns, requestID, reviewerID, note))
	if err != nil {
		return nil, fmt.Errorf("approve_publish_request: update: %w", err)
	}

	ch := req.ChapterNumber
	if err := insertChangelog(ctx, tx, ChangelogEntry{
		VersionID: req.VersionID, ChapterNumber: &ch, Action: ActionPublishApproved,
		ActorID: reviewerID, PublishRequestID: &req.ID,
	}); err != nil {
		return nil, err
	}
	if err := insertChangelog(ctx, tx, ChangelogEntry{
		VersionID: req.VersionID, ChapterNumber: &ch, Action: ActionPublished,
		ActorID: reviewerID, PublishRequestID: &req.ID,
		Detail: map[string]any{"verses_published": verses},
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("approve_publish_request: commit: %w", err)
	}
	return updated, nil
}

// RejectPublishRequest transitions a pending request to rejected.
func (db *DB) RejectPublishRequest(ctx context.Context, requestID, reviewerID int, note *string) (*PublishRequestRow, error) {
	return db.resolvePublishRequest(ctx, requestID, "rejected", ActionPublishRejected, &reviewerID, note)
}

// CancelPublishRequest transitions a pending request to cancelled.
func (db *DB) CancelPublishRequest(ctx context.Context, requestID, actorID int) (*PublishRequestRow, error) {
	return db.resolvePublishRequest(ctx, requestID, "cancelled", ActionPublishCancelled, &actorID, nil)
}

// resolvePublishRequest applies a terminal non-publishing transition.
func (db *DB) resolvePublishRequest(ctx context.Context, requestID int, status, action string, reviewerID *int, note *string) (*PublishRequestRow, error) {
	defer observe("resolve_publish_request", time.Now())
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	req, err := lockPendingRequest(ctx, tx, requestID)
	if err != nil {
		return nil, err
	}
	updated, err := scanPublishRequest(tx.QueryRow(ctx,
		`UPDATE quran_publish_requests
		 SET status=$2::quran_publish_status, reviewed_by=$3, reviewed_at=now(), review_note=$4, updated_at=now()
		 WHERE id=$1 RETURNING `+publishRequestColumns, requestID, status, reviewerID, note))
	if err != nil {
		return nil, fmt.Errorf("resolve_publish_request: update: %w", err)
	}
	ch := req.ChapterNumber
	if err := insertChangelog(ctx, tx, ChangelogEntry{
		VersionID: req.VersionID, ChapterNumber: &ch, Action: action,
		ActorID: derefOr(reviewerID, 0), PublishRequestID: &req.ID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("resolve_publish_request: commit: %w", err)
	}
	return updated, nil
}

// lockPendingRequest selects a request FOR UPDATE and verifies it is pending.
func lockPendingRequest(ctx context.Context, tx pgx.Tx, requestID int) (*PublishRequestRow, error) {
	req, err := scanPublishRequest(tx.QueryRow(ctx,
		`SELECT `+publishRequestColumns+` FROM quran_publish_requests WHERE id=$1 FOR UPDATE`, requestID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPublishRequestNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock_pending_request: %w", err)
	}
	if req.Status != "pending" {
		return nil, ErrPublishRequestNotPending
	}
	return req, nil
}

func chapterIDByNumber(ctx context.Context, tx pgx.Tx, chapterNumber int) (int, error) {
	var id int
	if err := tx.QueryRow(ctx, `SELECT id FROM chapters WHERE chapter_number=$1`, chapterNumber).Scan(&id); err != nil {
		return 0, fmt.Errorf("chapter_id_by_number: %w", err)
	}
	return id, nil
}

// syncVersePublished upserts every drafted verse of the chapter into the
// editorial published table and returns the number of verses published.
func syncVersePublished(ctx context.Context, tx pgx.Tx, versionID, chapterID, requestID int) (int, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO quran_verse_text_published
		    (version_id, verse_id, subtitle, content, footer, published_request_id, published_at)
		SELECT d.version_id, d.verse_id, d.subtitle, d.content, d.footer, $3, now()
		FROM verse_text_drafts d
		JOIN verses v ON v.id = d.verse_id
		WHERE d.version_id = $1 AND v.chapter_id = $2
		ON CONFLICT (version_id, verse_id) DO UPDATE
		    SET subtitle = EXCLUDED.subtitle,
		        content  = EXCLUDED.content,
		        footer   = EXCLUDED.footer,
		        published_request_id = EXCLUDED.published_request_id,
		        published_at = now()`, versionID, chapterID, requestID)
	if err != nil {
		return 0, fmt.Errorf("sync_verse_published: %w", err)
	}
	// Mark the synced drafts as published. Only flip rows still in 'draft' so an
	// already-published verse's updated_at (its audit timestamp) is preserved.
	if _, err := tx.Exec(ctx, `
		UPDATE verse_text_drafts d
		SET status = 'published', updated_at = now()
		FROM verses v
		WHERE v.id = d.verse_id AND d.version_id = $1 AND v.chapter_id = $2 AND d.status = 'draft'`, versionID, chapterID); err != nil {
		return 0, fmt.Errorf("sync_verse_published: mark drafts: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// syncChapterPublished upserts the chapter-title draft into the editorial
// published table (no-op if there is no title draft).
func syncChapterPublished(ctx context.Context, tx pgx.Tx, versionID, chapterID, requestID int) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO quran_chapter_text_published
		    (version_id, chapter_id, title, published_request_id, published_at)
		SELECT version_id, chapter_id, title, $3, now()
		FROM chapter_text_drafts
		WHERE version_id = $1 AND chapter_id = $2
		ON CONFLICT (version_id, chapter_id) DO UPDATE
		    SET title = EXCLUDED.title,
		        published_request_id = EXCLUDED.published_request_id,
		        published_at = now()`, versionID, chapterID, requestID); err != nil {
		return fmt.Errorf("sync_chapter_published: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE chapter_text_drafts SET status='published', updated_at=now()
		WHERE version_id=$1 AND chapter_id=$2 AND status='draft'`, versionID, chapterID); err != nil {
		return fmt.Errorf("sync_chapter_published: mark draft: %w", err)
	}
	return nil
}

const changelogInsertSQL = `
	INSERT INTO quran_editorial_changelog
	    (version_id, chapter_number, verse_number, action, actor_id, publish_request_id, detail)
	VALUES ($1, $2, $3, $4, $5, $6, $7)`

// insertChangelog appends one event within a transaction.
func insertChangelog(ctx context.Context, tx pgx.Tx, e ChangelogEntry) error {
	detail := []byte("{}")
	if e.Detail != nil {
		b, err := json.Marshal(e.Detail)
		if err != nil {
			return fmt.Errorf("insert_changelog: marshal detail: %w", err)
		}
		detail = b
	}
	if _, err := tx.Exec(ctx, changelogInsertSQL,
		e.VersionID, e.ChapterNumber, e.VerseNumber, e.Action, e.ActorID, e.PublishRequestID, detail); err != nil {
		return fmt.Errorf("insert_changelog: %w", err)
	}
	return nil
}

// AppendChangelog appends one event outside a transaction (draft saves).
func (db *DB) AppendChangelog(ctx context.Context, e ChangelogEntry) error {
	detail := []byte("{}")
	if e.Detail != nil {
		b, err := json.Marshal(e.Detail)
		if err != nil {
			return fmt.Errorf("append_changelog: marshal detail: %w", err)
		}
		detail = b
	}
	if _, err := db.Pool.Exec(ctx, changelogInsertSQL,
		e.VersionID, e.ChapterNumber, e.VerseNumber, e.Action, e.ActorID, e.PublishRequestID, detail); err != nil {
		return fmt.Errorf("append_changelog: %w", err)
	}
	return nil
}

// ListChangelog returns recent changelog entries for a version+chapter.
func (db *DB) ListChangelog(ctx context.Context, versionID, chapterNumber, limit int) ([]ChangelogRow, error) {
	defer observe("list_changelog", time.Now())
	rows, err := db.Pool.Query(ctx, `
		SELECT id, version_id, chapter_number, verse_number, action, actor_id,
		       publish_request_id, detail, created_at
		FROM quran_editorial_changelog
		WHERE version_id = $1 AND chapter_number = $2
		ORDER BY created_at DESC, id DESC
		LIMIT $3`, versionID, chapterNumber, limit)
	if err != nil {
		return nil, fmt.Errorf("list_changelog: %w", err)
	}
	defer rows.Close()
	var out []ChangelogRow
	for rows.Next() {
		var r ChangelogRow
		if err := rows.Scan(&r.ID, &r.VersionID, &r.ChapterNumber, &r.VerseNumber, &r.Action,
			&r.ActorID, &r.PublishRequestID, &r.Detail, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("list_changelog: scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func derefOr(p *int, fallback int) int {
	if p == nil {
		return fallback
	}
	return *p
}
