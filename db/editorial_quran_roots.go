package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// likeEscaper neutralizes LIKE metacharacters so a prefix filter matches the
// literal user input (paired with ESCAPE '\' in the query).
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// escapeLikePrefix returns q with LIKE wildcards escaped, or nil unchanged.
func escapeLikePrefix(q *string) *string {
	if q == nil {
		return nil
	}
	esc := likeEscaper.Replace(*q)
	return &esc
}

// Editorial Quran root meanings (Phase 2). A meaning is stored once per
// (version, root) in quran_root_meaning_drafts and rendered into the word table
// by JOIN, so one edit "broadcasts" to every word sharing the root. Root
// meanings span chapters, so they publish via a dedicated approver action into
// quran_root_meanings_published rather than the per-chapter publish workflow.

// QuranRootRow is the assembled per-root editing surface for a version.
type QuranRootRow struct {
	RootID           int
	Letters          string
	Occurrences      int
	MeaningDraft     *string
	DraftStatus      *string
	MeaningPublished *string
	ReferenceMeaning *string
}

// rootSelectColumns is shared by the list and single-root fetch. versionID is $1.
// The correlated reference subquery pulls the canonical English gloss for the
// root to translate from (mirrors RootDetail); it runs at most once per returned
// row and is backed by idx_word_root_lookup.
const rootSelectColumns = `
	r.id, r.letters,
	COALESCE(mv.occurrences, 0) AS occurrences,
	d.meaning AS meaning_draft, d.status::text AS draft_status,
	p.meaning AS meaning_published,
	(SELECT wt.meaning FROM word_text wt
	   JOIN languages l ON wt.lang_id = l.id AND l.code = 'en'
	   WHERE wt.word_root = r.letters AND wt.meaning IS NOT NULL AND wt.meaning <> ''
	   LIMIT 1) AS reference_meaning`

const rootSelectFrom = `
	FROM quran_roots r
	LEFT JOIN root_index_mv mv ON mv.letters = r.letters
	LEFT JOIN quran_root_meaning_drafts d ON d.root_id = r.id AND d.version_id = $1
	LEFT JOIN quran_root_meanings_published p ON p.root_id = r.id AND p.version_id = $1`

func scanRootRow(row pgx.Row) (QuranRootRow, error) {
	var r QuranRootRow
	err := row.Scan(&r.RootID, &r.Letters, &r.Occurrences,
		&r.MeaningDraft, &r.DraftStatus, &r.MeaningPublished, &r.ReferenceMeaning)
	return r, err
}

// ListRootMeanings returns a paginated, optionally prefix-filtered root list for
// a version, ordered by corpus frequency, with the filtered total.
func (db *DB) ListRootMeanings(ctx context.Context, versionID int, q *string, limit, offset int) ([]QuranRootRow, int, error) {
	defer observe("list_root_meanings", time.Now())
	prefix := escapeLikePrefix(q)
	rows, err := db.Pool.Query(ctx,
		`SELECT`+rootSelectColumns+rootSelectFrom+`
		 WHERE ($2::text IS NULL OR r.letters LIKE $2 || '%' ESCAPE '\')
		 ORDER BY COALESCE(mv.occurrences, 0) DESC, r.letters
		 LIMIT $3 OFFSET $4`,
		versionID, prefix, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list_root_meanings: %w", err)
	}
	defer rows.Close()
	var out []QuranRootRow
	for rows.Next() {
		r, err := scanRootRow(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("list_root_meanings: scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list_root_meanings: %w", err)
	}

	var total int
	if err := db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM quran_roots r WHERE ($1::text IS NULL OR r.letters LIKE $1 || '%' ESCAPE '\')`,
		prefix).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("list_root_meanings: count: %w", err)
	}
	return out, total, nil
}

// FetchRootMeaning returns one assembled root row for a version, or (nil, nil)
// when the root id is unknown.
func (db *DB) FetchRootMeaning(ctx context.Context, versionID, rootID int) (*QuranRootRow, error) {
	r, err := scanRootRow(db.Pool.QueryRow(ctx,
		`SELECT`+rootSelectColumns+rootSelectFrom+` WHERE r.id = $2`, versionID, rootID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetch_root_meaning: %w", err)
	}
	return &r, nil
}

// UpsertRootMeaningDraft creates or updates this version's draft meaning for a
// root and resets it to 'draft'.
func (db *DB) UpsertRootMeaningDraft(ctx context.Context, versionID, rootID int, meaning *string, authorID int) error {
	defer observe("upsert_root_meaning_draft", time.Now())
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO quran_root_meaning_drafts (version_id, root_id, meaning, status, author_id, updated_at)
		VALUES ($1, $2, $3, 'draft', $4, now())
		ON CONFLICT (version_id, root_id)
		DO UPDATE SET meaning = EXCLUDED.meaning,
		              status = 'draft',
		              author_id = EXCLUDED.author_id,
		              updated_at = now()`,
		versionID, rootID, meaning, authorID)
	if err != nil {
		return fmt.Errorf("upsert_root_meaning_draft: %w", err)
	}
	return nil
}

// PublishRootMeanings syncs every draft-status root meaning for a version into
// the published table and marks those drafts published, in one transaction, then
// records a changelog entry. Returns the number of meanings published.
func (db *DB) PublishRootMeanings(ctx context.Context, versionID, reviewerID int, note *string) (int, error) {
	defer observe("publish_root_meanings", time.Now())
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx, `
		INSERT INTO quran_root_meanings_published (version_id, root_id, meaning, published_at)
		SELECT version_id, root_id, meaning, now()
		FROM quran_root_meaning_drafts
		WHERE version_id = $1 AND status = 'draft'
		ON CONFLICT (version_id, root_id) DO UPDATE
		    SET meaning = EXCLUDED.meaning, published_at = now()`, versionID)
	if err != nil {
		return 0, fmt.Errorf("publish_root_meanings: sync: %w", err)
	}
	count := int(tag.RowsAffected())

	if _, err := tx.Exec(ctx, `
		UPDATE quran_root_meaning_drafts SET status = 'published', updated_at = now()
		WHERE version_id = $1 AND status = 'draft'`, versionID); err != nil {
		return 0, fmt.Errorf("publish_root_meanings: mark drafts: %w", err)
	}

	detail := map[string]any{"published": count}
	if note != nil {
		detail["note"] = *note
	}
	if err := insertChangelog(ctx, tx, ChangelogEntry{
		VersionID: versionID, Action: ActionRootMeaningsPublished,
		ActorID: reviewerID, Detail: detail,
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("publish_root_meanings: commit: %w", err)
	}
	return count, nil
}
