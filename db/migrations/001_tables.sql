-----------------------------------------
-- Types
-----------------------------------------

CREATE TABLE IF NOT EXISTS languages (
    id SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code VARCHAR(10) NOT NULL UNIQUE, -- 'en', 'ar', 'fr'
    name VARCHAR(50) NOT NULL,        -- 'English', 'Arabic'
    direction VARCHAR(3) DEFAULT 'ltr' -- 'ltr' or 'rtl'
);

CREATE TYPE translation_status AS ENUM ('draft', 'published', 'archived');

-----------------------------------------
-- Structural tables 
-----------------------------------------

CREATE TABLE IF NOT EXISTS chapters (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chapter_number int NOT NULL,
    verse_count int NOT NULL,
    revelation_order int NOT NULL
);

CREATE TABLE IF NOT EXISTS verses (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chapter_id INT,
    verse_index INT,
    CONSTRAINT fk_chapter
        FOREIGN KEY(chapter_id)
        REFERENCES chapters(id)
        ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS words (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    verse_id INT, 
    global_index INT, 
    word_index INT, 
    CONSTRAINT fk_verse
        FOREIGN KEY(verse_id)
        REFERENCES verses(id)
        ON DELETE CASCADE
);

-----------------------------------------
-- Content tables 
-----------------------------------------

CREATE TABLE IF NOT EXISTS chapter_text (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, 
    lang_id SMALLINT NOT NULL REFERENCES languages(id),
    chapter_id INT NOT NULL, 
    title TEXT,

    -- metadata
    published_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_chapter
        FOREIGN KEY(chapter_id)
        REFERENCES chapters(id)
        ON DELETE CASCADE,
    CONSTRAINT uq_chapter_text_lang UNIQUE(chapter_id, lang_id)
);

CREATE TABLE IF NOT EXISTS verse_text (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, 
    verse_id INT NOT NULL, 
    lang_id SMALLINT NOT NULL REFERENCES languages(id),
    subtitle TEXT, 
    content TEXT,
    footer TEXT,

    -- metadata
    published_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_verse
        FOREIGN KEY(verse_id)
        REFERENCES verses(id)
        ON DELETE CASCADE,
    CONSTRAINT uq_verse_text_lang UNIQUE(verse_id, lang_id)
    
);

CREATE TABLE IF NOT EXISTS word_text (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, 
    word_id INT NOT NULL,
    lang_id SMALLINT NOT NULL REFERENCES languages(id),
    word TEXT,
    word_root TEXT,

    -- metadata
    published_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_word
        FOREIGN KEY(word_id)
        REFERENCES words(id)
        ON DELETE CASCADE,
    CONSTRAINT uq_word_text_lang UNIQUE(word_id, lang_id)
);


-----------------------------------------
-- Draft tables 
-----------------------------------------
CREATE TABLE IF NOT EXISTS chapter_text_drafts (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chapter_id INT NOT NULL,
    lang_id SMALLINT NOT NULL REFERENCES languages(id),
    title TEXT,

    -- Versioning 
    version INT DEFAULT 1,
    status translation_status DEFAULT 'draft',

    -- meta data
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    -- author
    author_id INT,

    CONSTRAINT fk_chapter
            FOREIGN KEY(chapter_id)
            REFERENCES chapters(id)
            ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS verse_text_drafts (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    verse_id INT NOT NULL,
    lang_id SMALLINT NOT NULL REFERENCES languages(id),
    subtitle TEXT,
    content TEXT,
    footer TEXT,

    -- Versioning 
    version INT DEFAULT 1,
    status translation_status DEFAULT 'draft',

    -- meta data
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    -- author
    author_id INT,

    CONSTRAINT fk_verse
        FOREIGN KEY(verse_id)
        REFERENCES verses(id)
        ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS word_text_drafts (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    word_id INT NOT NULL,
    lang_id SMALLINT NOT NULL REFERENCES languages(id),
    word TEXT,
    word_root TEXT,

    -- Versioning 
    version INT DEFAULT 1,
    status translation_status DEFAULT 'draft',

    -- meta data
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    -- author
    author_id INT,

    CONSTRAINT fk_word
        FOREIGN KEY(word_id)
        REFERENCES words(id)
        ON DELETE CASCADE
);

-----------------------------------------
-- History tables 
-----------------------------------------

CREATE TABLE IF NOT EXISTS chapter_text_history (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chapter_id INT NOT NULL,
    chapter_text_id INT NOT NULL,
    lang_id SMALLINT NOT NULL REFERENCES languages(id),
    title TEXT,
    version INT,
    archived_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_chapter
        FOREIGN KEY(chapter_id)
        REFERENCES chapters(id)
        ON DELETE CASCADE,
    
    CONSTRAINT fk_chapter_text
        FOREIGN KEY(chapter_text_id)
        REFERENCES chapter_text(id)
        ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS verse_text_history (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    verse_text_id INT,
    verse_id INT NOT NULL,
    lang_id SMALLINT NOT NULL REFERENCES languages(id),
    content TEXT,
    subtitle TEXT,
    footer TEXT,
    version INT,
    archived_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    reason TEXT,

    CONSTRAINT fk_verse
        FOREIGN KEY(verse_id)
        REFERENCES verses(id)
        ON DELETE CASCADE,
    
    CONSTRAINT fk_verse_text
        FOREIGN KEY(verse_text_id)
        REFERENCES verse_text(id)
        ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS word_text_history (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    word_id INT NOT NULL,
    word_text_id INT NOT NULL,
    word TEXT,
    word_root TEXT,

    version INT,
    archived_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_word
        FOREIGN KEY(word_id)
        REFERENCES words(id)
        ON DELETE CASCADE,
    
    CONSTRAINT fk_word_text
        FOREIGN KEY(word_text_id)
        REFERENCES word_text(id)
        ON DELETE CASCADE
);