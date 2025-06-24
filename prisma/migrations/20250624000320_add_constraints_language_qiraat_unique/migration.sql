/*
  Warnings:

  - A unique constraint covering the columns `[language,qiraat]` on the table `Quran` will be added. If there are existing duplicate values, this will fail.

*/
-- CreateIndex
CREATE UNIQUE INDEX "Quran_language_qiraat_key" ON "Quran"("language", "qiraat");
