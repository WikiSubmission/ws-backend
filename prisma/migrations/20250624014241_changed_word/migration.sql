/*
  Warnings:

  - The values [null] on the enum `Qiraats` will be removed. If these variants are still used in the database, this will fail.
  - Added the required column `language` to the `Word` table without a default value. This is not possible if the table is not empty.

*/
-- AlterEnum
ALTER TYPE "Languages" ADD VALUE 'ARABIC_ROOT';

-- AlterEnum
BEGIN;
CREATE TYPE "Qiraats_new" AS ENUM ('HAFIZ', 'HAFIZH', 'NAFI_AL_MADANI', 'IBN_KATHIR_AL_MAKKI', 'ABU_AMR_AL_BASRI', 'IBN_AMIR_AD_DIMASHQI', 'ASIM_AL_KUFI', 'HAMZAH_AL_KUFI', 'AL_KISAI_AL_KUFI', 'ABU_JAFER_AL_MADANI', 'JAKOB_AL_HADRAMI', 'KHALAF_AL__ASHIR');
ALTER TABLE "Quran" ALTER COLUMN "qiraat" TYPE "Qiraats_new" USING ("qiraat"::text::"Qiraats_new");
ALTER TYPE "Qiraats" RENAME TO "Qiraats_old";
ALTER TYPE "Qiraats_new" RENAME TO "Qiraats";
DROP TYPE "Qiraats_old";
COMMIT;

-- AlterTable
ALTER TABLE "Word" ADD COLUMN     "language" "Languages" NOT NULL;
