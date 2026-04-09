-----------------------------------------
-- Music structural tables
-----------------------------------------

CREATE TABLE IF NOT EXISTS music_artists (
    id               INT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,
    description      TEXT,
    image_url        TEXT NOT NULL DEFAULT 'https://cdn.wikisubmission.org/media/brand-assets/logo-transparent.png',
    display_priority INT  NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS music_categories (
    id               INT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,
    description      TEXT,
    display_priority INT  NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS music_albums (
    id           INT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         TEXT NOT NULL,
    artist_id    INT  NOT NULL REFERENCES music_artists(id)  ON UPDATE CASCADE ON DELETE CASCADE,
    release_date DATE NOT NULL DEFAULT '2025-01-01',
    description  TEXT
);

CREATE TABLE IF NOT EXISTS music_tracks (
    id           INT     GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         TEXT    NOT NULL,
    release_date DATE    NOT NULL DEFAULT '2025-01-01',
    url          TEXT    NOT NULL,
    artist_id    INT     NOT NULL REFERENCES music_artists(id)   ON UPDATE CASCADE ON DELETE RESTRICT,
    category_id  INT     REFERENCES music_categories(id)         ON UPDATE CASCADE ON DELETE SET NULL,
    album_id     INT     REFERENCES music_albums(id)             ON UPDATE CASCADE ON DELETE SET NULL,
    featured     BOOLEAN NOT NULL DEFAULT FALSE,
    lyrics       TEXT
);

-----------------------------------------
-- Music indexes
-----------------------------------------

CREATE INDEX IF NOT EXISTS idx_music_tracks_artist   ON music_tracks(artist_id);
CREATE INDEX IF NOT EXISTS idx_music_tracks_category ON music_tracks(category_id);
CREATE INDEX IF NOT EXISTS idx_music_tracks_album    ON music_tracks(album_id);
CREATE INDEX IF NOT EXISTS idx_music_tracks_featured ON music_tracks(featured) WHERE featured = TRUE;
CREATE INDEX IF NOT EXISTS idx_music_albums_artist   ON music_albums(artist_id);
