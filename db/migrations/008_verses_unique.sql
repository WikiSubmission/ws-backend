-- Allow idempotent upserts on the verses table
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_verses_chapter_verse'
    ) THEN
        ALTER TABLE verses ADD CONSTRAINT uq_verses_chapter_verse UNIQUE (chapter_id, verse_index);
    END IF;
END $$;
