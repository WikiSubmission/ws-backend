/*
  Warnings:

  - A unique constraint covering the columns `[global_index,chapter_index,verse_index,language]` on the table `Verses` will be added. If there are existing duplicate values, this will fail.
  - A unique constraint covering the columns `[global_index,chapter_index,verse_index,word_index,language]` on the table `Words` will be added. If there are existing duplicate values, this will fail.
  - Added the required column `root` to the `Words` table without a default value. This is not possible if the table is not empty.

*/
-- AlterTable
ALTER TABLE "Words" ADD COLUMN     "root" TEXT NOT NULL;

-- CreateIndex
CREATE UNIQUE INDEX "Verses_global_index_chapter_index_verse_index_language_key" ON "Verses"("global_index", "chapter_index", "verse_index", "language");

-- CreateIndex
CREATE UNIQUE INDEX "Words_global_index_chapter_index_verse_index_word_index_lan_key" ON "Words"("global_index", "chapter_index", "verse_index", "word_index", "language");
