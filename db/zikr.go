package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type ZikrRow struct {
	ID        int       `db:"id"`
	Text      string    `db:"text"`
	Lang      string    `db:"lang"`
	Source    *string   `db:"source"`
	Active    bool      `db:"active"`
	Weight    int       `db:"weight"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

const zikrCols = `id, text, lang, source, active, weight, created_at, updated_at`

// ListZikr returns zikr entries ordered for display. When activeOnly is set
// (the public endpoint) inactive rows are excluded; the admin surface lists
// everything. lang filters when non-empty.
func (db *DB) ListZikr(ctx context.Context, activeOnly bool, lang string) ([]ZikrRow, error) {
	query := `SELECT ` + zikrCols + ` FROM zikr_entries WHERE ($1 = false OR active) AND ($2 = '' OR lang = $2) ORDER BY weight, id`
	rows, err := db.Pool.Query(ctx, query, activeOnly, lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[ZikrRow])
}

func (db *DB) InsertZikr(ctx context.Context, text, lang string, source *string, active bool, weight int) (*ZikrRow, error) {
	query := `INSERT INTO zikr_entries (text, lang, source, active, weight)
	          VALUES ($1, $2, $3, $4, $5)
	          RETURNING ` + zikrCols
	rows, err := db.Pool.Query(ctx, query, text, lang, source, active, weight)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ZikrRow])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// UpdateZikr applies a partial update; nil fields keep their current value.
// Returns nil when the id does not exist.
func (db *DB) UpdateZikr(ctx context.Context, id int, text, lang, source *string, active *bool, weight *int) (*ZikrRow, error) {
	query := `UPDATE zikr_entries SET
	            text = COALESCE($2, text),
	            lang = COALESCE($3, lang),
	            source = COALESCE($4, source),
	            active = COALESCE($5, active),
	            weight = COALESCE($6, weight),
	            updated_at = NOW()
	          WHERE id = $1
	          RETURNING ` + zikrCols
	rows, err := db.Pool.Query(ctx, query, id, text, lang, source, active, weight)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ZikrRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// DeleteZikr removes an entry. Returns false when nothing matched.
func (db *DB) DeleteZikr(ctx context.Context, id int) (bool, error) {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM zikr_entries WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
