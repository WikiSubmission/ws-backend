package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Row structs
// ---------------------------------------------------------------------------

type MusicArtistRow struct {
	ID              int     `db:"id"`
	Name            string  `db:"name"`
	Description     *string `db:"description"`
	ImageURL        string  `db:"image_url"`
	DisplayPriority int     `db:"display_priority"`
}

type MusicCategoryRow struct {
	ID              int     `db:"id"`
	Name            string  `db:"name"`
	Description     *string `db:"description"`
	DisplayPriority int     `db:"display_priority"`
}

type MusicAlbumRow struct {
	ID          int     `db:"id"`
	Name        string  `db:"name"`
	ArtistID    int     `db:"artist_id"`
	ReleaseDate string  `db:"release_date"`
	Description *string `db:"description"`
}

type MusicTrackRow struct {
	ID          int     `db:"id"`
	Name        string  `db:"name"`
	ReleaseDate string  `db:"release_date"`
	URL         string  `db:"url"`
	ArtistID    int     `db:"artist_id"`
	CategoryID  *int    `db:"category_id"`
	AlbumID     *int    `db:"album_id"`
	Featured    bool    `db:"featured"`
	Lyrics      *string `db:"lyrics"`
}

// ---------------------------------------------------------------------------
// Query functions
// ---------------------------------------------------------------------------

func (db *DB) FetchMusicArtists(ctx context.Context) ([]MusicArtistRow, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, name, description, image_url, display_priority
		FROM music_artists
		ORDER BY display_priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[MusicArtistRow])
}

func (db *DB) FetchMusicCategories(ctx context.Context) ([]MusicCategoryRow, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, name, description, display_priority
		FROM music_categories
		ORDER BY display_priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[MusicCategoryRow])
}

func (db *DB) FetchMusicAlbums(ctx context.Context, artistID *int) ([]MusicAlbumRow, error) {
	query := `
		SELECT id, name, artist_id, release_date::text, description
		FROM music_albums`
	args := []any{}
	if artistID != nil {
		query += fmt.Sprintf(" WHERE artist_id = $%d", len(args)+1)
		args = append(args, *artistID)
	}
	query += " ORDER BY release_date DESC, id ASC"

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[MusicAlbumRow])
}

func (db *DB) FetchMusicTracks(ctx context.Context, artistID *int, categoryID *int, albumID *int, featured *bool) ([]MusicTrackRow, error) {
	query := `
		SELECT id, name, release_date::text, url, artist_id, category_id, album_id, featured, lyrics
		FROM music_tracks
		WHERE TRUE`
	args := []any{}
	if artistID != nil {
		args = append(args, *artistID)
		query += fmt.Sprintf(" AND artist_id = $%d", len(args))
	}
	if categoryID != nil {
		args = append(args, *categoryID)
		query += fmt.Sprintf(" AND category_id = $%d", len(args))
	}
	if albumID != nil {
		args = append(args, *albumID)
		query += fmt.Sprintf(" AND album_id = $%d", len(args))
	}
	if featured != nil {
		args = append(args, *featured)
		query += fmt.Sprintf(" AND featured = $%d", len(args))
	}
	query += " ORDER BY release_date DESC, id ASC"

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[MusicTrackRow])
}
