/* eslint-disable @typescript-eslint/no-misused-promises */
import { NestFactory } from '@nestjs/core';
import { AppModule } from 'src/app.module';
import { PrismaService } from 'src/modules';
import * as fs from 'fs';
import { parse } from 'csv-parse';
import * as cliProgress from 'cli-progress';

type WsQuran = {
  global_index: number; // bigint -> number in TS
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

export async function bootstrapW() {
  const app = await NestFactory.createApplicationContext(AppModule);
  const prisma = app.get(PrismaService);

  try {
    console.log('Starting Quran CSV import...');
    const csvFilePath = 'prisma/seeds/data/ws-quran_rows.csv';

    const rowCount = await countCsvRows(csvFilePath);
    console.log('csv rows are: ', rowCount);
    const progressBar = new cliProgress.SingleBar(
      {},
      cliProgress.Presets.shades_classic,
    );
    progressBar.start(rowCount, 0);

    const arabic_tashkeel = await prisma.quran.create({
      data: { language: 'ARABIC_TASHKEEL' },
    });

    const arabic_no_tashkeel = await prisma.quran.create({
      data: { language: 'ARABIC_NON_TASHKEEL' },
    });

    const english = await prisma.quran.create({
      data: { language: 'ENGLISH' },
    });

    const transliteration = await prisma.quran.create({
      data: { language: 'TRANSLITERATED' },
    });

    const parser = fs
      .createReadStream(csvFilePath)
      .pipe(parse({ columns: true }));
    let processedRows = 0;
    for await (const row of parser) {
      const typedRow = row as WsQuran;
      const chapterNumber = Number(typedRow.chapter_number);
      const chapterRevelationOrder = Number(typedRow.chapter_revelation_order);

      const chapterEnglishTitle = typedRow.chapter_title_english;
      const chapterArabicTashkeelTitle = typedRow.chapter_title_arabic;
      const chapterArabicNoTashkeelTitle = typedRow.chapter_title_arabic;
      const chapterTransliterationTitle = typedRow.chapter_title_transliterated;

      const verseNumber = Number(typedRow.verse_number);
      const verseContentEnglish = typedRow.verse_text_english;
      const verseContentArabicTashkeel = typedRow.verse_text_arabic;
      const verseContentArabicNoTashkeel = typedRow.verse_text_arabic_clean;
      const verseContentTransliteration = typedRow.verse_text_transliterated;
      const verseSubtitleEnglish = typedRow.verse_subtitle_english;
      const verseFootnoteEnglish = typedRow.verse_footnote_english;

      const englishChapter = await prisma.chapter.upsert({
        where: {
          quranId_number: {
            number: chapterNumber,
            quranId: english.id,
          },
        },
        create: {
          quran: { connect: { id: english.id } },
          number: chapterNumber,
          name: chapterEnglishTitle,
          revelation_order: chapterRevelationOrder,
        },
        update: {},
      });

      const arabicTashkeelChapter = await prisma.chapter.upsert({
        where: {
          quranId_number: {
            number: chapterNumber,
            quranId: arabic_tashkeel.id,
          },
        },
        create: {
          quran: { connect: { id: arabic_tashkeel.id } },
          number: chapterNumber,
          name: chapterArabicTashkeelTitle,
          revelation_order: chapterRevelationOrder,
        },
        update: {},
      });

      const arabicNoTashkeelChapter = await prisma.chapter.upsert({
        where: {
          quranId_number: {
            number: chapterNumber,
            quranId: arabic_no_tashkeel.id,
          },
        },
        create: {
          quran: { connect: { id: arabic_no_tashkeel.id } },
          number: chapterNumber,
          name: chapterArabicNoTashkeelTitle,
          revelation_order: chapterRevelationOrder,
        },
        update: {},
      });

      const transliterationChapter = await prisma.chapter.upsert({
        where: {
          quranId_number: {
            number: chapterNumber,
            quranId: transliteration.id,
          },
        },
        create: {
          quran: { connect: { id: transliteration.id } },
          number: chapterNumber,
          name: chapterTransliterationTitle,
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

      processedRows += 1;
      progressBar.update(processedRows);
    }

    progressBar.stop();
    console.log('Finished import');
    await app.close();
  } catch (error) {
    console.error('Error while importing.');
    console.error(error);
    await app.close();
    process.exit(1);
  }
}

async function countCsvRows(filePath: string): Promise<number> {
  return new Promise((resolve, reject) => {
    let count = 0;
    fs.createReadStream(filePath)
      .pipe(parse({ columns: true }))
      .on('data', () => {
        count++;
      })
      .on('end', () => {
        resolve(count);
      })
      .on('error', (err) => {
        reject(err);
      });
  });
}
