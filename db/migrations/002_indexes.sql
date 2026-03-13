-----------------------------------------
-- Structural indexes
-----------------------------------------

-- Fetching a chapter using the chapter name
CREATE UNIQUE INDEX IF NOT EXISTS idx_chapters_number ON chapters(chapter_number);

-- Fetching verse(s) from chapter
CREATE INDEX IF NOT EXISTS idx_verses_nav ON verses(chapter_id, verse_index);

-- Word retrieval inside from a verse 
CREATE INDEX IF NOT EXISTS idx_words_nav ON words(verse_id, word_index);

-----------------------------------------
-- Content indexes 
-----------------------------------------

CREATE INDEX IF NOT EXISTS idx_chapter_text_lang ON chapter_text(chapter_id, lang_id);
CREATE INDEX IF NOT EXISTS idx_verse_text_lang ON verse_text(verse_id, lang_id);
CREATE INDEX IF NOT EXISTS idx_word_text_lang ON word_text(word_id, lang_id);

-- for fetching chapter/verse/word text for the verse struct
CREATE INDEX IF NOT EXISTS idx_word_root_lookup ON word_text(word_root) WHERE word_root IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_word_meaning_lookup ON word_text(meaning) WHERE meaning IS NOT NULL;

-- searching by root


-----------------------------------------
-- Draft indexes 
-----------------------------------------

-- Fast lookup for drafts by language or status
CREATE INDEX IF NOT EXISTS idx_verse_drafts_nav ON verse_text_drafts(verse_id, lang_id, status);


-----------------------------------------
-- History indexes 
-----------------------------------------

-- audit logs
CREATE INDEX IF NOT EXISTS idx_verse_history_nav ON verse_text_history(verse_id, lang_id, archived_at);
