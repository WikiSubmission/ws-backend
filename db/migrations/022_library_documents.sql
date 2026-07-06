-- Library documents: full text of the Introduction, Proclamation, and the 38
-- appendices, split into sections for full-text search (the web/mobile reader
-- renders its own copy of the content; these rows power /library/search and
-- the offline `library` bundle). Seeded from db/seeds/library_docs_en.json,
-- which is generated from the frontend content by
-- org/apps/web/scripts/extract-library-text.tsx.

CREATE TABLE IF NOT EXISTS library_documents (
    id         INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    doc_type   TEXT NOT NULL CHECK (doc_type IN ('appendix', 'introduction', 'proclamation')),
    doc_number INT,          -- appendix number; NULL for introduction/proclamation
    lang       TEXT NOT NULL DEFAULT 'en',
    title      TEXT NOT NULL
);

-- doc_number is NULL for the two single documents, so a plain UNIQUE would
-- admit duplicates; COALESCE(0) makes NULL collide like a real value.
CREATE UNIQUE INDEX IF NOT EXISTS library_documents_identity_idx
    ON library_documents (doc_type, COALESCE(doc_number, 0), lang);

CREATE TABLE IF NOT EXISTS library_sections (
    id            INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    document_id   INT NOT NULL REFERENCES library_documents(id) ON DELETE CASCADE,
    section_index INT NOT NULL,
    heading       TEXT,
    body          TEXT NOT NULL,
    -- Same 'simple' configuration as verse_text.content_tsv: exact-token
    -- matching, no language stemming surprises across mixed content.
    content_tsv   tsvector GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(heading, '') || ' ' || body)
    ) STORED,
    UNIQUE (document_id, section_index)
);

CREATE INDEX IF NOT EXISTS library_sections_content_tsv_idx
    ON library_sections USING GIN (content_tsv);
