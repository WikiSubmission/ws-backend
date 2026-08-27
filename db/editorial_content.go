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

// First-party editorial content store (Sanity deprecation). One row per
// document: `draft` is the working copy edited in /editor, `published` is the
// snapshot readers see. Status is always derived, never stored — see
// migrations/024_editorial_content.sql.

var (
	// ErrContentNotFound signals a missing document (HTTP 404).
	ErrContentNotFound = errors.New("content document not found")
	// ErrContentConflict signals a uniqueness collision, e.g. duplicate slug (HTTP 409).
	ErrContentConflict = errors.New("content document conflicts with an existing one")
	// ErrContentBadReference signals a broken reference, e.g. unknown author id (HTTP 400).
	ErrContentBadReference = errors.New("content document references a missing row")
	// ErrCategoryDeleteForbidden means a non-admin tried to delete a category
	// created by another account (including legacy rows with no owner).
	ErrCategoryDeleteForbidden = errors.New("category delete requires its creator")
	// ErrCategoryInUse means at least one article still names the category.
	ErrCategoryInUse = errors.New("category is assigned to an article")
)

// ContentStatus is the derived lifecycle state of a document.
type ContentStatus string

const (
	ContentStatusDraft     ContentStatus = "draft"
	ContentStatusChanged   ContentStatus = "changed"
	ContentStatusPublished ContentStatus = "published"
)

// ContentDocRow is one editorial content document.
type ContentDocRow struct {
	ID               int
	Status           ContentStatus
	TranslationGroup *string
	Draft            json.RawMessage
	Published        json.RawMessage
	CreatedAt        time.Time
	UpdatedAt        time.Time
	PublishedAt      *time.Time
	UpdatedByEmail   *string
}

// ContentScalars carries the payload fields that are mirrored into real
// columns for uniqueness and filtering. The handler extracts and validates
// them from the draft payload; only the fields relevant to the module are set.
type ContentScalars struct {
	Slug      *string // article, author, category, community
	LangID    *int    // article
	AuthorID  *int    // article
	UserID    *int    // author (optional WS account link)
	VersionID *int    // appendix
	Code      *string // appendix
}

// contentModuleSpec describes the storage shape of one content module.
type contentModuleSpec struct {
	table string
	// scalarCols maps insertable/updatable column names to an extractor.
	scalarCols []string
	// labelExpr is a SQL expression over alias d used for q-substring search.
	labelExpr string
	hasGroup  bool
}

var contentModuleSpecs = map[EditorModule]contentModuleSpec{
	ModuleArticle: {
		table:      "editorial_articles",
		scalarCols: []string{"slug", "lang_id", "author_id"},
		labelExpr:  "coalesce(d.slug,'') || ' ' || coalesce(d.draft->>'title','')",
		hasGroup:   true,
	},
	ModuleAuthor: {
		table:      "editorial_authors",
		scalarCols: []string{"slug", "user_id"},
		labelExpr:  "coalesce(d.slug,'') || ' ' || coalesce(d.draft->>'first_name','') || ' ' || coalesce(d.draft->>'last_name','')",
	},
	ModuleCategory: {
		table:      "editorial_categories",
		scalarCols: []string{"slug"},
		labelExpr:  "coalesce(d.slug,'') || ' ' || coalesce(d.draft->>'name','')",
	},
	ModuleCommunity: {
		table:      "editorial_communities",
		scalarCols: []string{"slug"},
		labelExpr:  "coalesce(d.slug,'') || ' ' || coalesce(d.draft->>'name','')",
	},
	ModuleAppendix: {
		table:      "editorial_appendices",
		scalarCols: []string{"version_id", "code"},
		labelExpr:  "coalesce(d.code,'') || ' ' || coalesce(d.draft->>'title','')",
	},
}

func (s *ContentScalars) value(col string) any {
	switch col {
	case "slug":
		return s.Slug
	case "lang_id":
		return s.LangID
	case "author_id":
		return s.AuthorID
	case "user_id":
		return s.UserID
	case "version_id":
		return s.VersionID
	case "code":
		return s.Code
	default:
		return nil
	}
}

const contentStatusExpr = `CASE
	WHEN d.published IS NULL THEN 'draft'
	WHEN d.draft IS DISTINCT FROM d.published THEN 'changed'
	ELSE 'published'
END`

// groupExpr returns the translation_group select expression for the module.
func (spec contentModuleSpec) groupExpr() string {
	if spec.hasGroup {
		return "d.translation_group::text"
	}
	return "NULL::text"
}

func contentSpec(module EditorModule) (contentModuleSpec, error) {
	spec, ok := contentModuleSpecs[module]
	if !ok {
		return contentModuleSpec{}, fmt.Errorf("unknown content module %q", module)
	}
	return spec, nil
}

// translateContentPgError maps constraint violations onto the typed sentinel
// errors so handlers can answer 409/400 instead of 500.
func translateContentPgError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrContentConflict
		case "23503":
			return ErrContentBadReference
		}
	}
	return err
}

// ListContentDocs returns documents of one module ordered by last update,
// with the total match count before limit/offset.
func (db *DB) ListContentDocs(
	ctx context.Context,
	module EditorModule,
	q string,
	status ContentStatus,
	limit, offset int,
) ([]ContentDocRow, int, error) {
	defer observe("list_content_docs", time.Now())
	spec, err := contentSpec(module)
	if err != nil {
		return nil, 0, err
	}

	where := []string{"TRUE"}
	args := []any{}
	if q != "" {
		args = append(args, "%"+q+"%")
		where = append(where, fmt.Sprintf("(%s) ILIKE $%d", spec.labelExpr, len(args)))
	}
	if status != "" {
		args = append(args, string(status))
		where = append(where, fmt.Sprintf("(%s) = $%d", contentStatusExpr, len(args)))
	}
	args = append(args, limit, offset)

	query := fmt.Sprintf(`
		SELECT d.id, %s AS status, %s AS translation_group,
		       d.draft, d.published, d.created_at, d.updated_at, d.published_at,
		       u.email AS updated_by_email,
		       COUNT(*) OVER() AS total
		FROM %s d
		LEFT JOIN users u ON u.id = d.updated_by
		WHERE %s
		ORDER BY d.updated_at DESC, d.id DESC
		LIMIT $%d OFFSET $%d`,
		contentStatusExpr, spec.groupExpr(), spec.table,
		strings.Join(where, " AND "), len(args)-1, len(args),
	)

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []ContentDocRow
	total := 0
	for rows.Next() {
		var r ContentDocRow
		if err := rows.Scan(
			&r.ID, &r.Status, &r.TranslationGroup,
			&r.Draft, &r.Published, &r.CreatedAt, &r.UpdatedAt, &r.PublishedAt,
			&r.UpdatedByEmail, &total,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// GetContentDoc fetches one document, or ErrContentNotFound.
func (db *DB) GetContentDoc(ctx context.Context, module EditorModule, id int) (*ContentDocRow, error) {
	defer observe("get_content_doc", time.Now())
	spec, err := contentSpec(module)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		SELECT d.id, %s AS status, %s AS translation_group,
		       d.draft, d.published, d.created_at, d.updated_at, d.published_at,
		       u.email AS updated_by_email
		FROM %s d
		LEFT JOIN users u ON u.id = d.updated_by
		WHERE d.id = $1`,
		contentStatusExpr, spec.groupExpr(), spec.table,
	)
	var r ContentDocRow
	err = db.Pool.QueryRow(ctx, query, id).Scan(
		&r.ID, &r.Status, &r.TranslationGroup,
		&r.Draft, &r.Published, &r.CreatedAt, &r.UpdatedAt, &r.PublishedAt,
		&r.UpdatedByEmail,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrContentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// AuthorAccountID returns the WikiSubmission account an author row is linked
// to, or nil when it is linked to none. Read from the mirrored column rather
// than the draft payload: the column is what an admin set, so an editor cannot
// claim someone else's row by editing their own draft JSON.
//
// Returns ErrContentNotFound when no such author exists.
func (db *DB) AuthorAccountID(ctx context.Context, id int) (*int, error) {
	defer observe("author_account_id", time.Now())
	var userID *int
	err := db.Pool.QueryRow(ctx,
		`SELECT user_id FROM editorial_authors WHERE id = $1`, id,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrContentNotFound
	}
	if err != nil {
		return nil, err
	}
	return userID, nil
}

// CreateContentDoc inserts a new draft document. translationGroup only applies
// to modules with hasGroup (articles); pass nil to start a fresh group.
func (db *DB) CreateContentDoc(
	ctx context.Context,
	module EditorModule,
	draft json.RawMessage,
	scalars ContentScalars,
	translationGroup *string,
	actorID int,
) (*ContentDocRow, error) {
	defer observe("create_content_doc", time.Now())
	spec, err := contentSpec(module)
	if err != nil {
		return nil, err
	}

	cols := []string{"draft", "created_by", "updated_by"}
	args := []any{draft, actorID, actorID}
	for _, col := range spec.scalarCols {
		cols = append(cols, col)
		args = append(args, scalars.value(col))
	}
	if spec.hasGroup && translationGroup != nil {
		cols = append(cols, "translation_group")
		args = append(args, *translationGroup)
	}
	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	query := fmt.Sprintf(
		`INSERT INTO %s (%s) VALUES (%s) RETURNING id`,
		spec.table, strings.Join(cols, ", "), strings.Join(placeholders, ", "),
	)
	var id int
	if err := db.Pool.QueryRow(ctx, query, args...).Scan(&id); err != nil {
		return nil, translateContentPgError(err)
	}
	return db.GetContentDoc(ctx, module, id)
}

// UpdateContentDoc replaces the working draft (and mirrored scalar columns) of
// one document. The published snapshot is untouched.
func (db *DB) UpdateContentDoc(
	ctx context.Context,
	module EditorModule,
	id int,
	draft json.RawMessage,
	scalars ContentScalars,
	actorID int,
) (*ContentDocRow, error) {
	defer observe("update_content_doc", time.Now())
	spec, err := contentSpec(module)
	if err != nil {
		return nil, err
	}

	sets := []string{"draft = $1", "updated_by = $2", "updated_at = now()"}
	args := []any{draft, actorID}
	for _, col := range spec.scalarCols {
		args = append(args, scalars.value(col))
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	args = append(args, id)

	query := fmt.Sprintf(
		`UPDATE %s SET %s WHERE id = $%d`,
		spec.table, strings.Join(sets, ", "), len(args),
	)
	tag, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, translateContentPgError(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrContentNotFound
	}
	return db.GetContentDoc(ctx, module, id)
}

// PublishContentDoc copies the draft into the published snapshot.
func (db *DB) PublishContentDoc(ctx context.Context, module EditorModule, id, actorID int) (*ContentDocRow, error) {
	defer observe("publish_content_doc", time.Now())
	return db.setPublished(ctx, module, id, actorID, true)
}

// UnpublishContentDoc retracts the published snapshot; the document keeps its
// draft and returns to draft status.
func (db *DB) UnpublishContentDoc(ctx context.Context, module EditorModule, id, actorID int) (*ContentDocRow, error) {
	defer observe("unpublish_content_doc", time.Now())
	return db.setPublished(ctx, module, id, actorID, false)
}

func (db *DB) setPublished(ctx context.Context, module EditorModule, id, actorID int, publish bool) (*ContentDocRow, error) {
	spec, err := contentSpec(module)
	if err != nil {
		return nil, err
	}
	set := "published = draft, published_at = now()"
	if !publish {
		set = "published = NULL, published_at = NULL"
	}
	query := fmt.Sprintf(
		`UPDATE %s SET %s, updated_by = $1, updated_at = now() WHERE id = $2`,
		spec.table, set,
	)
	tag, err := db.Pool.Exec(ctx, query, actorID, id)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrContentNotFound
	}
	return db.GetContentDoc(ctx, module, id)
}

// DeleteContentDoc removes the document, draft and snapshot alike.
func (db *DB) DeleteContentDoc(ctx context.Context, module EditorModule, id int) error {
	defer observe("delete_content_doc", time.Now())
	spec, err := contentSpec(module)
	if err != nil {
		return err
	}
	tag, err := db.Pool.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, spec.table), id)
	if err != nil {
		return translateContentPgError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrContentNotFound
	}
	return nil
}

// DeleteCategory enforces the non-admin ownership and usage rules in one
// transaction. The table lock prevents an article update from assigning the
// category between the usage check and delete; category deletion is rare, so
// correctness is worth the brief write serialization here.
func (db *DB) DeleteCategory(ctx context.Context, id, actorID int, isAdmin bool) error {
	defer observe("delete_category", time.Now())
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var createdBy *int
	if err := tx.QueryRow(ctx,
		`SELECT created_by FROM editorial_categories WHERE id = $1 FOR UPDATE`, id,
	).Scan(&createdBy); errors.Is(err, pgx.ErrNoRows) {
		return ErrContentNotFound
	} else if err != nil {
		return err
	}

	if !isAdmin {
		if createdBy == nil || *createdBy != actorID {
			return ErrCategoryDeleteForbidden
		}
		if _, err := tx.Exec(ctx, `LOCK TABLE editorial_articles IN SHARE MODE`); err != nil {
			return err
		}
		var inUse bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM editorial_articles
				WHERE COALESCE(draft->'categories', '[]'::jsonb) @> jsonb_build_array($1::integer)
				   OR COALESCE(published->'categories', '[]'::jsonb) @> jsonb_build_array($1::integer)
			)`, id).Scan(&inUse); err != nil {
			return err
		}
		if inUse {
			return ErrCategoryInUse
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM editorial_categories WHERE id = $1`, id); err != nil {
		return translateContentPgError(err)
	}
	return tx.Commit(ctx)
}
