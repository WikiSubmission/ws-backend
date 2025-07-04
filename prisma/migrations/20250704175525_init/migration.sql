-- CreateEnum
CREATE TYPE "Languages" AS ENUM ('ARABIC_TASHKEEL', 'ARABIC_NON_TASHKEEL', 'TRANSLITERATED', 'ENGLISH', 'FR_NCH', 'TAMIL', 'HINDI', 'TURKISH', 'PERSIAN', 'RUSSIAN', 'SWEDISH', 'GERMAN', 'BAHASA', 'ARABIC_ROOT');

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
    "footer" TEXT NOT NULL,
    "language" "Languages" NOT NULL,
    "chapter_index" INTEGER NOT NULL,
    "chapter_title" TEXT NOT NULL,

    CONSTRAINT "Verses_pkey" PRIMARY KEY ("uuid")
);

-- CreateTable
CREATE TABLE "Words" (
    "uuid" TEXT NOT NULL,
    "global_index" SERIAL NOT NULL,
    "word" INTEGER NOT NULL,
    "chapter_index" INTEGER NOT NULL,
    "verse_index" INTEGER NOT NULL,
    "word_index" INTEGER NOT NULL,
    "content" TEXT NOT NULL,
    "language" "Languages" NOT NULL,

    CONSTRAINT "Words_pkey" PRIMARY KEY ("uuid")
);

-- CreateTable
CREATE TABLE "ChapterMetadata" (
    "chapter_idex" INTEGER NOT NULL,
    "global_index" INTEGER NOT NULL,
    "title" TEXT NOT NULL,
    "verse_count" INTEGER NOT NULL,
    "revelation_order" INTEGER NOT NULL,

    CONSTRAINT "ChapterMetadata_pkey" PRIMARY KEY ("chapter_idex")
);

-- CreateTable
CREATE TABLE "Gematria" (
    "letter" TEXT NOT NULL,
    "value" INTEGER NOT NULL,

    CONSTRAINT "Gematria_pkey" PRIMARY KEY ("letter")
);
