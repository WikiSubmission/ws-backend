-- AlterEnum
-- This migration adds more than one value to an enum.
-- With PostgreSQL versions 11 and earlier, this is not possible
-- in a single migration. This can be worked around by creating
-- multiple migrations, each migration adding only one value to
-- the enum.


ALTER TYPE "Languages" ADD VALUE 'FR_NCH';
ALTER TYPE "Languages" ADD VALUE 'TAMIL';
ALTER TYPE "Languages" ADD VALUE 'HINDI';
ALTER TYPE "Languages" ADD VALUE 'TURKISH';
ALTER TYPE "Languages" ADD VALUE 'PERSIAN';

-- AlterEnum
-- This migration adds more than one value to an enum.
-- With PostgreSQL versions 11 and earlier, this is not possible
-- in a single migration. This can be worked around by creating
-- multiple migrations, each migration adding only one value to
-- the enum.


ALTER TYPE "Qiraats" ADD VALUE 'NAFI_AL_MADANI';
ALTER TYPE "Qiraats" ADD VALUE 'IBN_KATHIR_AL_MAKKI';
ALTER TYPE "Qiraats" ADD VALUE 'ABU_AMR_AL_BASRI';
ALTER TYPE "Qiraats" ADD VALUE 'IBN_AMIR_AD_DIMASHQI';
ALTER TYPE "Qiraats" ADD VALUE 'ASIM_AL_KUFI';
ALTER TYPE "Qiraats" ADD VALUE 'HAMZAH_AL_KUFI';
ALTER TYPE "Qiraats" ADD VALUE 'AL_KISAI_AL_KUFI';
ALTER TYPE "Qiraats" ADD VALUE 'ABU_JAFER_AL_MADANI';
ALTER TYPE "Qiraats" ADD VALUE 'JAKOB_AL_HADRAMI';
ALTER TYPE "Qiraats" ADD VALUE 'KHALAF_AL__ASHIR';
