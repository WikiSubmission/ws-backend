-----------------------------------------
-- Bible structural tables
-----------------------------------------

CREATE TABLE IF NOT EXISTS bible_books (
    id SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_number SMALLINT NOT NULL UNIQUE,
    name TEXT NOT NULL UNIQUE,
    testament VARCHAR(2) NOT NULL DEFAULT 'OT' CHECK (testament IN ('OT', 'NT'))
);

CREATE TABLE IF NOT EXISTS bible_chapters (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id SMALLINT NOT NULL,
    chapter_number SMALLINT NOT NULL,
    verse_count INT NOT NULL DEFAULT 0,
    CONSTRAINT fk_bible_book
        FOREIGN KEY(book_id)
        REFERENCES bible_books(id)
        ON DELETE CASCADE,
    CONSTRAINT uq_bible_chapter UNIQUE (book_id, chapter_number)
);

CREATE TABLE IF NOT EXISTS bible_verses (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chapter_id INT NOT NULL,
    verse_number SMALLINT NOT NULL,
    CONSTRAINT fk_bible_chapter
        FOREIGN KEY(chapter_id)
        REFERENCES bible_chapters(id)
        ON DELETE CASCADE,
    CONSTRAINT uq_bible_verse UNIQUE (chapter_id, verse_number)
);

-----------------------------------------
-- Bible content tables
-----------------------------------------

CREATE TABLE IF NOT EXISTS bible_verse_text (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    verse_id INT NOT NULL,
    lang_id SMALLINT NOT NULL REFERENCES languages(id),
    content         TEXT,
    footer          TEXT,
    theo_footnotes  TEXT[],
    content_tsv TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('simple',
            coalesce(content, '') || ' ' || coalesce(footer, '')
        )
    ) STORED,
    published_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_bible_verse
        FOREIGN KEY(verse_id)
        REFERENCES bible_verses(id)
        ON DELETE CASCADE,
    CONSTRAINT uq_bible_verse_text_lang UNIQUE (verse_id, lang_id)
);

-- For future multilingual book titles
CREATE TABLE IF NOT EXISTS bible_book_text (
    id SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id SMALLINT NOT NULL,
    lang_id SMALLINT NOT NULL REFERENCES languages(id),
    name TEXT NOT NULL,
    CONSTRAINT fk_bible_book_text
        FOREIGN KEY(book_id)
        REFERENCES bible_books(id)
        ON DELETE CASCADE,
    CONSTRAINT uq_bible_book_text_lang UNIQUE (book_id, lang_id)
);

-----------------------------------------
-- Bible indexes
-----------------------------------------

CREATE INDEX IF NOT EXISTS idx_bible_chapters_book ON bible_chapters(book_id, chapter_number);
CREATE INDEX IF NOT EXISTS idx_bible_verses_nav ON bible_verses(chapter_id, verse_number);
CREATE INDEX IF NOT EXISTS idx_bible_verse_text_lang ON bible_verse_text(verse_id, lang_id);
CREATE INDEX IF NOT EXISTS idx_bible_verse_fts ON bible_verse_text USING GIN(content_tsv);
CREATE INDEX IF NOT EXISTS idx_bible_verse_content_trgm ON bible_verse_text USING GIN(content gin_trgm_ops);
