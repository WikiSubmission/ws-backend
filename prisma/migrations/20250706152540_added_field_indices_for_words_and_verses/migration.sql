-- CreateIndex
CREATE INDEX "Verses_chapter_index_verse_index_language_idx" ON "Verses"("chapter_index", "verse_index", "language");

-- CreateIndex
CREATE INDEX "Words_chapter_index_verse_index_versesUuid_language_idx" ON "Words"("chapter_index", "verse_index", "versesUuid", "language");
