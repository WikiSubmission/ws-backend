-- Allow idempotent upserts on the words table
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_words_verse_word'
    ) THEN
        ALTER TABLE words ADD CONSTRAINT uq_words_verse_word UNIQUE (verse_id, word_index);
    END IF;
END $$;
