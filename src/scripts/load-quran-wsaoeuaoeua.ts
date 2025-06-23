/* eslint-disable @typescript-eslint/no-misused-promises */
import { NestFactory } from '@nestjs/core';
import { AppModule } from 'src/app.module';
import { PrismaService } from 'src/prisma/prisma.service';
import * as fs from 'fs';
import { parse } from 'csv-parse';
import { Quran } from '@prisma/client';

export type WsQuran = {
  global_index: number;
  verse_index: number;
  chapter_number: number;
  verse_id: string;
  verse_id_arabic: string;
  verse_number: number;
  verse_text_english: string;
  verse_text_arabic: string;
  verse_text_transliterated: string;
  verse_subtitle_english: string | null;
  verse_footnote_english: string | null;
  verse_text_arabic_clean: string;
  chapter_title_english: string;
  chapter_title_arabic: string;
  chapter_title_transliterated: string;
  chapter_revelation_order: number;
};

async function bootstrap() {
  const app = await NestFactory.createApplicationContext(AppModule);
  const prisma = app.get(PrismaService);

  try {
    console.log('Starting Quran CSV import...');
    const csvFilePath =
      '/home/hichem/Projects/wikisubmission-backend-quran/data/ws-quran_rows.csv';

    const arabic_tashkeel = await prisma.quran.create({ data: { language: 'ARABIC_TASHKEEL' } });
    const arabic_no_tashkeel = await prisma.quran.create({ data: { language: 'ARABIC_NON_TASHKEEL' } });
    const english = await prisma.quran.create({ data: { language: 'ENGLISH' } });
    const transliteration = await prisma.quran.create({ data: { language: 'TRANSLITERATED' } });

    const parser = fs.createReadStream(csvFilePath).pipe(parse({ columns: true }));

    for await (const row of parser) {
      const typedRow = row as WsQuran;

      const chapterNumber = Number(typedRow.chapter_number);
      const chapterRevelationOrder = Number(typedRow.chapter_revelation_order);

      const verseNumber = Number(typedRow.verse_number);
      const verseContentEnglish = typedRow.verse_text_english;
      const verseContentArabicTashkeel = typedRow.verse_text_arabic;
      const verseContentArabicNoTashkeel = typedRow.verse_text_arabic_clean;
      const verseContentTransliteration = typedRow.verse_text_transliterated;
      const verseSubtitleEnglish = typedRow.verse_subtitle_english;
      const verseFootnoteEnglish = typedRow.verse_footnote_english;

      const englishChapter = await prisma.chapter.upsert({
        where: { quranId_number: { number: chapterNumber, quranId: english.id } },
        create: {
          quran: { connect: { id: english.id } },
          number: chapterNumber,
          name: typedRow.chapter_title_english,
          revelation_order: chapterRevelationOrder,
        },
        update: {},
      });

      const arabicTashkeelChapter = await prisma.chapter.upsert({
        where: { quranId_number: { number: chapterNumber, quranId: arabic_tashkeel.id } },
        create: {
          quran: { connect: { id: arabic_tashkeel.id } },
          number: chapterNumber,
          name: typedRow.chapter_title_arabic,
          revelation_order: chapterRevelationOrder,
        },
        update: {},
      });

      const arabicNoTashkeelChapter = await prisma.chapter.upsert({
        where: { quranId_number: { number: chapterNumber, quranId: arabic_no_tashkeel.id } },
        create: {
          quran: { connect: { id: arabic_no_tashkeel.id } },
          number: chapterNumber,
          name: typedRow.chapter_title_arabic,
          revelation_order: chapterRevelationOrder,
        },
        update: {},
      });

      const transliterationChapter = await prisma.chapter.upsert({
        where: { quranId_number: { number: chapterNumber, quranId: transliteration.id } },
        create: {
          quran: { connect: { id: transliteration.id } },
          number: chapterNumber,
          name: typedRow.chapter_title_transliterated,
          revelation_order: chapterRevelationOrder,
        },
        update: {},
      });

      await prisma.verse.create({
        data: {
          chapter: { connect: { id: englishChapter.id } },
          number: verseNumber,
          subtitle: verseSubtitleEnglish,
          content: verseContentEnglish,
          footnote: verseFootnoteEnglish,
        },
      });

      await prisma.verse.create({
        data: {
          chapter: { connect: { id: arabicTashkeelChapter.id } },
          number: verseNumber,
          subtitle: null,
          content: verseContentArabicTashkeel,
          footnote: null,
        },
      });

      await prisma.verse.create({
        data: {
          chapter: { connect: { id: arabicNoTashkeelChapter.id } },
          number: verseNumber,
          subtitle: null,
          content: verseContentArabicNoTashkeel,
          footnote: null,
        },
      });

      await prisma.verse.create({
        data: {
          chapter: { connect: { id: transliterationChapter.id } },
          number: verseNumber,
          subtitle: null,
          content: verseContentTransliteration,
          footnote: null,
        },
      });
    }

    console.log('Finished import');
    await app.close();
  } catch (error) {
    console.error('Error while importing.');
    console.error(error);
    await app.close();
    process.exit(1);
  }
}

bootstrap();
