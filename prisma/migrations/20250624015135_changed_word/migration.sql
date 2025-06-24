/*
  Warnings:

  - You are about to drop the column `language` on the `Word` table. All the data in the column will be lost.
  - Added the required column `root` to the `Word` table without a default value. This is not possible if the table is not empty.

*/
-- AlterTable
ALTER TABLE "Word" DROP COLUMN "language",
ADD COLUMN     "root" TEXT NOT NULL;
