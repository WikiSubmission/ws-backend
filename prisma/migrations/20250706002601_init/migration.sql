-- CreateEnum
CREATE TYPE "Languages" AS ENUM ('ARABIC_TASHKEEL', 'ARABIC_NON_TASHKEEL', 'TRANSLITERATED', 'ENGLISH', 'FRENCH', 'TAMIL', 'HINDI', 'TURKISH', 'PERSIAN', 'RUSSIAN', 'SWEDISH', 'GERMAN', 'BAHASA', 'ARABIC_ROOT');

-- CreateEnum
CREATE TYPE "Qiraats" AS ENUM ('HAFIZ', 'HAFIZH', 'NAFI_AL_MADANI', 'IBN_KATHIR_AL_MAKKI', 'ABU_AMR_AL_BASRI', 'IBN_AMIR_AD_DIMASHQI', 'ASIM_AL_KUFI', 'HAMZAH_AL_KUFI', 'AL_KISAI_AL_KUFI', 'ABU_JAFER_AL_MADANI', 'JAKOB_AL_HADRAMI', 'KHALAF_AL__ASHIR');

-- CreateEnum
CREATE TYPE "Scripts" AS ENUM ('UTHMANI', 'UTHMANI_SIMPLE', 'IMLAEI', 'IMLAEI_SIMPLE');

-- CreateTable
CREATE TABLE "Verses" (
    "uuid" TEXT NOT NULL,
    "global_index" SERIAL NOT NULL,
    "verse_index" INTEGER NOT NULL,
    "subtitle" TEXT,
    "content" TEXT NOT NULL,
    "footer" TEXT,
    "language" "Languages" NOT NULL,
    "chapter_index" INTEGER NOT NULL,
    "chapter_title" TEXT NOT NULL,

    CONSTRAINT "Verses_pkey" PRIMARY KEY ("uuid")
);

-- CreateTable
CREATE TABLE "Words" (
    "uuid" TEXT NOT NULL,
    "global_index" SERIAL NOT NULL,
    "chapter_index" INTEGER NOT NULL,
    "verse_index" INTEGER NOT NULL,
    "word_index" INTEGER NOT NULL,
    "content" TEXT NOT NULL,
    "root" TEXT NOT NULL,
    "language" "Languages" NOT NULL,
    "versesUuid" TEXT NOT NULL,

    CONSTRAINT "Words_pkey" PRIMARY KEY ("uuid")
);

-- CreateTable
CREATE TABLE "ChapterMetadata" (
    "chapter_index" INTEGER NOT NULL,
    "global_index" INTEGER NOT NULL,
    "title_english" TEXT NOT NULL,
    "title_arabic" TEXT NOT NULL,
    "title_transliterated" TEXT NOT NULL,
    "verse_count" INTEGER NOT NULL,
    "revelation_order" INTEGER NOT NULL,

    CONSTRAINT "ChapterMetadata_pkey" PRIMARY KEY ("chapter_index")
);

-- CreateTable
CREATE TABLE "Gematria" (
    "letter" TEXT NOT NULL,
    "value" INTEGER NOT NULL,

    CONSTRAINT "Gematria_pkey" PRIMARY KEY ("letter")
);

-- CreateIndex
CREATE UNIQUE INDEX "Verses_global_index_chapter_index_verse_index_language_key" ON "Verses"("global_index", "chapter_index", "verse_index", "language");

-- CreateIndex
CREATE UNIQUE INDEX "Words_global_index_chapter_index_verse_index_word_index_lan_key" ON "Words"("global_index", "chapter_index", "verse_index", "word_index", "language");

-- AddForeignKey
ALTER TABLE "Words" ADD CONSTRAINT "Words_versesUuid_fkey" FOREIGN KEY ("versesUuid") REFERENCES "Verses"("uuid") ON DELETE RESTRICT ON UPDATE CASCADE;

-- CreateIndex
CREATE INDEX "Verses_chapter_index_verse_index_language_idx" ON "Verses"("chapter_index", "verse_index", "language");

-- CreateIndex
CREATE INDEX "Words_chapter_index_verse_index_versesUuid_language_idx" ON "Words"("chapter_index", "verse_index", "versesUuid", "language");
