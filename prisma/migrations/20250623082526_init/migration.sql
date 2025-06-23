-- CreateEnum
CREATE TYPE "Languages" AS ENUM ('ARABIC', 'ENGLISH');

-- CreateEnum
CREATE TYPE "Qiraats" AS ENUM ('HAFIZ', 'HAFIZH');

-- CreateEnum
CREATE TYPE "Bitrates" AS ENUM ('32', '40', '48', '64', '128', '192', '320');

-- CreateTable
CREATE TABLE "Quran" (
    "id" TEXT NOT NULL,
    "language" "Languages" NOT NULL,
    "qiraat" "Qiraats" NOT NULL,

    CONSTRAINT "Quran_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Chapter" (
    "id" TEXT NOT NULL,
    "quranId" TEXT NOT NULL,
    "number" INTEGER NOT NULL DEFAULT 1,
    "name" TEXT NOT NULL,
    "revelation_order" INTEGER NOT NULL,

    CONSTRAINT "Chapter_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Verse" (
    "id" TEXT NOT NULL,
    "chapterId" TEXT NOT NULL,
    "number" INTEGER NOT NULL DEFAULT 1,
    "content" TEXT NOT NULL,

    CONSTRAINT "Verse_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Word" (
    "id" TEXT NOT NULL,
    "verseId" TEXT NOT NULL,
    "number" INTEGER NOT NULL,
    "content" TEXT NOT NULL,

    CONSTRAINT "Word_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Audio" (
    "id" TEXT NOT NULL,
    "quranId" TEXT NOT NULL,
    "reciter" TEXT NOT NULL DEFAULT 'Unknown',
    "root" TEXT NOT NULL,
    "edition" TEXT NOT NULL,
    "bitrate" "Bitrates" NOT NULL,

    CONSTRAINT "Audio_pkey" PRIMARY KEY ("id")
);

-- AddForeignKey
ALTER TABLE "Chapter" ADD CONSTRAINT "Chapter_quranId_fkey" FOREIGN KEY ("quranId") REFERENCES "Quran"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Verse" ADD CONSTRAINT "Verse_chapterId_fkey" FOREIGN KEY ("chapterId") REFERENCES "Chapter"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Word" ADD CONSTRAINT "Word_verseId_fkey" FOREIGN KEY ("verseId") REFERENCES "Verse"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Audio" ADD CONSTRAINT "Audio_quranId_fkey" FOREIGN KEY ("quranId") REFERENCES "Quran"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
