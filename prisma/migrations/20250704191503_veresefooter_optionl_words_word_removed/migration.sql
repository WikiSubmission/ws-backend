/*
  Warnings:

  - You are about to drop the column `word` on the `Words` table. All the data in the column will be lost.

*/
-- AlterTable
ALTER TABLE "Verses" ALTER COLUMN "footer" DROP NOT NULL;

-- AlterTable
ALTER TABLE "Words" DROP COLUMN "word";
