/*
  Warnings:

  - You are about to drop the `Audio` table. If the table is not empty, all the data it contains will be lost.
  - Added the required column `metaDataId` to the `Quran` table without a default value. This is not possible if the table is not empty.

*/
-- CreateEnum
CREATE TYPE "Scripts" AS ENUM ('UTHMANI', 'UTHMANI_SIMPLE', 'IMLAEI', 'IMLAEI_SIMPLE');

-- DropForeignKey
ALTER TABLE "Audio" DROP CONSTRAINT "Audio_quranId_fkey";

-- AlterTable
ALTER TABLE "Quran" ADD COLUMN     "Script" "Scripts",
ADD COLUMN     "metaDataId" TEXT NOT NULL;

-- DropTable
DROP TABLE "Audio";

-- DropEnum
DROP TYPE "Bitrates";

-- CreateTable
CREATE TABLE "MetaData" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,

    CONSTRAINT "MetaData_pkey" PRIMARY KEY ("id")
);

-- AddForeignKey
ALTER TABLE "Quran" ADD CONSTRAINT "Quran_metaDataId_fkey" FOREIGN KEY ("metaDataId") REFERENCES "MetaData"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
