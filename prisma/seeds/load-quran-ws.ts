import { NestFactory } from '@nestjs/core';
import { AppModule } from '../../src/app.module';
import { PrismaService } from '@modules';
import * as fs from 'fs';
import { parse } from 'csv-parse';
import * as cliProgress from 'cli-progress';
import { Verses } from '@prisma/client';

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

    const parser = fs
      .createReadStream(csvFilePath)
      .pipe(parse({ columns: true }));
    let processedRows = 0;
    let dataInsertionList: (Omit<Verses, 'uuid' | 'subtitle' | 'footer'> &
      Partial<Pick<Verses, 'subtitle' | 'footer'>>)[] = [];
    for await (const row of parser) {
      const typedRow = row as WsQuran;
      const global_index = Number(typedRow.global_index);
      const chapterNumber = Number(typedRow.chapter_number);
      const chapterTitleArabic = typedRow.chapter_title_arabic;
      const chapterTitleEnglish = typedRow.chapter_title_english;
      const chatpterTitleTransliterated = typedRow.chapter_title_transliterated;

      const verseNumber = Number(typedRow.verse_number);
      const verseContentEnglish = typedRow.verse_text_english;
      const verseContentArabicTashkeel = typedRow.verse_text_arabic;
      const verseContentArabicNoTashkeel = typedRow.verse_text_arabic_clean;
      const verseContentTransliteration = typedRow.verse_text_transliterated;
      const verseSubtitleEnglish = typedRow.verse_subtitle_english;
      const verseFootnoteEnglish = typedRow.verse_footnote_english;

      await prisma.chapterMetadata.upsert({
        create: {
          chapter_index: chapterNumber,
          global_index: global_index,
          title_english: typedRow.chapter_title_english,
          title_arabic: typedRow.chapter_title_arabic,
          title_transliterated: typedRow.chapter_title_transliterated,
          verse_count: 1,
          revelation_order: Number(typedRow.chapter_revelation_order),
        },
        update: {
          verse_count: {
            increment: 1,
          },
        },
        where: { chapter_index: chapterNumber },
      });

      dataInsertionList = dataInsertionList.concat([
        {
          global_index,
          chapter_index: chapterNumber,
          verse_index: verseNumber,
          subtitle: verseSubtitleEnglish,
          content: verseContentEnglish,
          footer: verseFootnoteEnglish,
          language: 'ENGLISH',
          chapter_title: chapterTitleEnglish,
        },
        {
          global_index,
          chapter_index: chapterNumber,
          verse_index: verseNumber,
          content: verseContentTransliteration,
          language: 'TRANSLITERATED',
          chapter_title: chatpterTitleTransliterated,
        },
        {
          global_index,
          chapter_index: chapterNumber,
          verse_index: verseNumber,
          content: verseContentArabicNoTashkeel,
          language: 'ARABIC_NON_TASHKEEL',
          chapter_title: chapterTitleArabic,
        },
        {
          global_index,
          chapter_index: chapterNumber,
          verse_index: verseNumber,
          content: verseContentArabicTashkeel,
          language: 'ARABIC_TASHKEEL',
          chapter_title: chapterTitleArabic,
        },
      ]);
      // await prisma.verses.createMany({
      // data:
      // });

      processedRows += 1;
      progressBar.update(processedRows);
    }
    await prisma.verses.createMany({ data: dataInsertionList });

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
