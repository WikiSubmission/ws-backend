/*
  Warnings:

  - The values [ARABIC] on the enum `Languages` will be removed. If these variants are still used in the database, this will fail.
  - Added the required column `hizb` to the `Chapter` table without a default value. This is not possible if the table is not empty.

*/
-- AlterEnum
BEGIN;
CREATE TYPE "Languages_new" AS ENUM ('ARABIC_TASHKEEL', 'ARABCI_NON_TASHKEEL', 'ENGLISH', 'TRANSLITERATED');
ALTER TABLE "Quran" ALTER COLUMN "language" TYPE "Languages_new" USING ("language"::text::"Languages_new");
ALTER TYPE "Languages" RENAME TO "Languages_old";
ALTER TYPE "Languages_new" RENAME TO "Languages";
DROP TYPE "Languages_old";
COMMIT;

-- AlterTable
ALTER TABLE "Chapter" ADD COLUMN     "hizb" INTEGER NOT NULL;

-- AlterTable
ALTER TABLE "Verse" ADD COLUMN     "footnote" TEXT,
ADD COLUMN     "subtitle" TEXT;
