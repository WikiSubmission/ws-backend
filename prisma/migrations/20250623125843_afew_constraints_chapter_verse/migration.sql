/*
  Warnings:

  - The values [ARABCI_NON_TASHKEEL] on the enum `Languages` will be removed. If these variants are still used in the database, this will fail.
  - A unique constraint covering the columns `[quranId,number]` on the table `Chapter` will be added. If there are existing duplicate values, this will fail.
  - A unique constraint covering the columns `[chapterId,number]` on the table `Verse` will be added. If there are existing duplicate values, this will fail.

*/
-- AlterEnum
BEGIN;
CREATE TYPE "Languages_new" AS ENUM ('ARABIC_TASHKEEL', 'ARABIC_NON_TASHKEEL', 'TRANSLITERATED', 'ENGLISH', 'FR_NCH', 'TAMIL', 'HINDI', 'TURKISH', 'PERSIAN');
ALTER TABLE "Quran" ALTER COLUMN "language" TYPE "Languages_new" USING ("language"::text::"Languages_new");
ALTER TYPE "Languages" RENAME TO "Languages_old";
ALTER TYPE "Languages_new" RENAME TO "Languages";
DROP TYPE "Languages_old";
COMMIT;

-- CreateIndex
CREATE UNIQUE INDEX "Chapter_quranId_number_key" ON "Chapter"("quranId", "number");

-- CreateIndex
CREATE UNIQUE INDEX "Verse_chapterId_number_key" ON "Verse"("chapterId", "number");
