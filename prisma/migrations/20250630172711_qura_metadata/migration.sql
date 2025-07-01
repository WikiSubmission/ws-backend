-- DropForeignKey
ALTER TABLE "Quran" DROP CONSTRAINT "Quran_metaDataId_fkey";

-- AlterTable
ALTER TABLE "Quran" ALTER COLUMN "metaDataId" DROP NOT NULL;

-- AddForeignKey
ALTER TABLE "Quran" ADD CONSTRAINT "Quran_metaDataId_fkey" FOREIGN KEY ("metaDataId") REFERENCES "MetaData"("id") ON DELETE SET NULL ON UPDATE CASCADE;
