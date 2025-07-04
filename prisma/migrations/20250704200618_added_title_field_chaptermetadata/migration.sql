/*
  Warnings:

  - You are about to drop the column `title` on the `ChapterMetadata` table. All the data in the column will be lost.
  - Added the required column `title_arabic` to the `ChapterMetadata` table without a default value. This is not possible if the table is not empty.
  - Added the required column `title_english` to the `ChapterMetadata` table without a default value. This is not possible if the table is not empty.
  - Added the required column `title_transliterated` to the `ChapterMetadata` table without a default value. This is not possible if the table is not empty.

*/
-- AlterTable
ALTER TABLE "ChapterMetadata" DROP COLUMN "title",
ADD COLUMN     "title_arabic" TEXT NOT NULL,
ADD COLUMN     "title_english" TEXT NOT NULL,
ADD COLUMN     "title_transliterated" TEXT NOT NULL;
