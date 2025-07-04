/*
  Warnings:

  - The values [FR_NCH] on the enum `Languages` will be removed. If these variants are still used in the database, this will fail.

*/
-- AlterEnum
BEGIN;
CREATE TYPE "Languages_new" AS ENUM ('ARABIC_TASHKEEL', 'ARABIC_NON_TASHKEEL', 'TRANSLITERATED', 'ENGLISH', 'FRENCH', 'TAMIL', 'HINDI', 'TURKISH', 'PERSIAN', 'RUSSIAN', 'SWEDISH', 'GERMAN', 'BAHASA', 'ARABIC_ROOT');
ALTER TABLE "Verses" ALTER COLUMN "language" TYPE "Languages_new" USING ("language"::text::"Languages_new");
ALTER TABLE "Words" ALTER COLUMN "language" TYPE "Languages_new" USING ("language"::text::"Languages_new");
ALTER TYPE "Languages" RENAME TO "Languages_old";
ALTER TYPE "Languages_new" RENAME TO "Languages";
DROP TYPE "Languages_old";
COMMIT;
