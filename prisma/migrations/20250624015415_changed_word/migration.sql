/*
  Warnings:

  - Added the required column `quran_index` to the `Word` table without a default value. This is not possible if the table is not empty.

*/
-- AlterTable
ALTER TABLE "Word" ADD COLUMN     "quran_index" INTEGER NOT NULL;
