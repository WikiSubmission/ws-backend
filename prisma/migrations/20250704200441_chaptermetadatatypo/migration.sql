/*
  Warnings:

  - The primary key for the `ChapterMetadata` table will be changed. If it partially fails, the table could be left without primary key constraint.
  - You are about to drop the column `chapter_idex` on the `ChapterMetadata` table. All the data in the column will be lost.
  - Added the required column `chapter_index` to the `ChapterMetadata` table without a default value. This is not possible if the table is not empty.

*/
-- AlterTable
ALTER TABLE "ChapterMetadata" DROP CONSTRAINT "ChapterMetadata_pkey",
DROP COLUMN "chapter_idex",
ADD COLUMN     "chapter_index" INTEGER NOT NULL,
ADD CONSTRAINT "ChapterMetadata_pkey" PRIMARY KEY ("chapter_index");
