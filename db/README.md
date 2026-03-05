# Database Architecture & Reference
This database is designed to power a high-performance, multi-language Quranic datastructure. It follows Third Normal Form (3NF) for structural data while employing a State-Isolated strategy for content to balance lightning-fast public reads with a robust editorial workflow.

# Architecture Layers
The database is divided into three distinct layers to ensure concerns remain separated:

- **Structural Layer:** The "Skeleton." This contains the immutable structure of the Quran (Chapters, Verses, and Words).

- **Content Layer (Live):** The "Source of Truth." Optimized exclusively for the public API. It contains only the currently published translations.

- **Governance Layer (Drafts & History):** The "Engine Room." This handles the messy reality of content creation, versioning, rollbacks, and audit trails without slowing down the public API.

# Table Definitions & Purposes
## 1. Structural Tables
chapters: Stores the metadata for each Surah (number, verse count, revelation order).

verses: The central pivot of the database. All content and structural elements link back to a verse_id.

words: Enables word-by-word analysis. Each word is mapped to a verse and maintains its own word_index and global_index.

## 2. Content Tables (Live)
These tables are intentionally kept "lean." We removed versioning flags (like is_current) in favor of a One-Row-Per-Item-Per-Language rule.

chapter_text / verse_text / word_text: These contain the active strings.

Integrity: Each table uses a UNIQUE(item_id, lang_id) constraint. This prevents data corruption where the API might accidentally find two "active" versions for a single language.

## 3. Draft & History Tables (Governance)
To keep the public API fast, we moved work-in-progress and old data to dedicated tables:

Drafts (_drafts): Where editors create and iterate on content. Includes version, status (Enum: draft, published, archived), and author_id.

History (_history): An append-only audit log. Every time a live version is updated, the previous version is moved here. This allows for instant rollbacks and a full historical record of changes.

# Indexing & Performance Strategy
Performance is built into the schema through two primary methods: Composite Navigation and Physical Clustering.

## 1. Navigation & Lookup
We use Composite Indexes to ensure the database can find data in a single "hop."

idx_verses_nav (chapter_id, verse_index): Optimized for fetching a range of verses (e.g., "Give me Surah 2, Verses 1–10").

idx_verse_text_lookup (verse_id, lang_id): Optimized for the API to fetch a specific translation without scanning the whole table.

## 2. Partial Indexes
For the `word_text` table, we use a Partial Index on `word_root`:

`CREATE INDEX idx_word_root_lookup ON word_text(word_root) WHERE word_root IS NOT NULL;`
This keeps the index small by excluding common words (prepositions/conjunctions) that don't have a linguistic root, making root-based searches incredibly fast.

## 3. Physical Clustering
The database uses the CLUSTER command:

`CLUSTER verses USING idx_verses_nav`: This physically rearranges the data on the disk to match the order of the Quran. This means when the API asks for Verse 1, Verse 2 is physically located right next to it on the SSD, reducing read latency to the absolute minimum.

# Internationalization (i18n)
Instead of hardcoding language strings, we use a normalized languages table.

`lang_id`: All content tables reference an integer ID. This is 50% faster to index than a string (like 'en').

`direction`: Stores rtl (Right-to-Left) or ltr (Left-to-Right). This allows the frontend to automatically adjust the UI layout (Arabic vs. English) based on the database response.

# Content Lifecycle (The "Publish" Flow)
When a version is ready to go live, the following logic should be executed in a Transaction:

Archive: Select the current row from verse_text and insert it into verse_text_history.

Promote: Update verse_text with the new content from verse_text_drafts.

Cleanup: Update the draft status to published.

This ensures the public API is never "blank" during an update and maintains a perfect 1:1 relationship between IDs and versions.

# Schema files
`001_tables.sql` to build the schema.

`002_indexes.sql` to optimize for search.

`003_seed.sql` to populate languages and perform the initial disk clustering.