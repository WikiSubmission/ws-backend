import { NestFactory } from '@nestjs/core';
import { AppModule } from '../../src/app.module';
import { PrismaService } from '@modules';
import * as fs from 'fs';
import { parse } from 'csv-parse';
import * as cliProgress from 'cli-progress';
import { Verses } from '@prisma/client';

export type WsQuranForeign = {
  verse_id: string;

  verse_text_french: string;
  verse_subtitle_french: string | null;
  verse_footnote_french: string | null;
  chapter_title_french: string;

  verse_text_swedish: string;
  verse_subtitle_swedish: string | null;
  verse_footnote_swedish: string | null;
  chapter_title_swedish: string;

  verse_text_russian: string;
  verse_subtitle_russian: string | null;
  verse_footnote_russian: string | null;
  chapter_title_russian: string;

  verse_text_turkish: string;
  verse_subtitle_turkish: string | null;
  verse_footnote_turkish: string | null;
  chapter_title_turkish: string;

  verse_text_german: string;
  verse_subtitle_german: string | null;
  verse_footnote_german: string | null;
  chapter_title_german: string;

  verse_text_bahasa: string;
  verse_subtitle_bahasa: string | null;
  verse_footnote_bahasa: string | null;
  chapter_title_bahasa: string;

  verse_text_persian: string;
  verse_subtitle_persian: string | null;
  verse_footnote_persian: string | null;
  chapter_title_persian: string;

  verse_text_tamil: string;
  verse_subtitle_tamil: string | null;
  verse_footnote_tamil: string | null;
  chapter_title_tamil: string;
};

export async function bootstrapQT() {
  const app = await NestFactory.createApplicationContext(AppModule);
  const prisma = app.get(PrismaService);

  try {
    console.log('Starting translated Quran CSV import...');
    const csvFilePath = 'prisma/seeds/data/ws-quran-foreign_rows.csv';

    const rowCount = await countCsvRows(csvFilePath);
    console.log('csv rows are: ', rowCount);
    const progressBar = new cliProgress.SingleBar(
      {},
      cliProgress.Presets.shades_classic,
    );

    progressBar.start(rowCount, 0);
    let processedRows = 0;
    const parser = fs
      .createReadStream(csvFilePath)
      .pipe(parse({ columns: true }));

    let verseList: (Omit<Verses, 'uuid' | 'subtitle' | 'footer'> &
      Partial<Pick<Verses, 'subtitle' | 'footer'>>)[] = [];
    for await (const untypedRow of parser) {
      const row = untypedRow as WsQuranForeign;
      const [chapter, verse] = row.verse_id.split(':').map(Number);

      const og = await prisma.verses.findFirstOrThrow({
        where: {
          verse_index: verse,
          chapter_index: chapter,
        },
      });

      verseList = verseList.concat([
        {
          global_index: og.global_index,
          chapter_index: chapter,
          verse_index: verse,
          subtitle: row.verse_subtitle_bahasa,
          content: row.verse_text_bahasa,
          footer: row.verse_footnote_bahasa,
          language: 'BAHASA',
          chapter_title: row.chapter_title_bahasa,
        },
        {
          global_index: og.global_index,
          chapter_index: chapter,
          verse_index: verse,
          subtitle: row.verse_subtitle_french,
          content: row.verse_text_french,
          footer: row.verse_footnote_french,
          language: 'FRENCH',
          chapter_title: row.chapter_title_french,
        },
        {
          global_index: og.global_index,
          chapter_index: chapter,
          verse_index: verse,
          subtitle: row.verse_subtitle_persian,
          content: row.verse_text_persian,
          footer: row.verse_footnote_persian,
          language: 'PERSIAN',
          chapter_title: row.chapter_title_persian,
        },
        {
          global_index: og.global_index,
          chapter_index: chapter,
          verse_index: verse,
          subtitle: row.verse_subtitle_russian,
          content: row.verse_text_russian,
          footer: row.verse_footnote_russian,
          language: 'RUSSIAN',
          chapter_title: row.chapter_title_russian,
        },
        {
          global_index: og.global_index,
          chapter_index: chapter,
          verse_index: verse,
          subtitle: row.verse_subtitle_swedish,
          content: row.verse_text_swedish,
          footer: row.verse_footnote_swedish,
          language: 'SWEDISH',
          chapter_title: row.chapter_title_swedish,
        },
        {
          global_index: og.global_index,
          chapter_index: chapter,
          verse_index: verse,
          subtitle: row.verse_subtitle_german,
          content: row.verse_text_german,
          footer: row.verse_footnote_german,
          language: 'GERMAN',
          chapter_title: row.chapter_title_german,
        },
        {
          global_index: og.global_index,
          chapter_index: chapter,
          verse_index: verse,
          subtitle: row.verse_subtitle_tamil,
          content: row.verse_text_tamil,
          footer: row.verse_footnote_tamil,
          language: 'TAMIL',
          chapter_title: row.chapter_title_tamil,
        },
        {
          global_index: og.global_index,
          chapter_index: chapter,
          verse_index: verse,
          subtitle: row.verse_subtitle_turkish,
          content: row.verse_text_turkish,
          footer: row.verse_footnote_turkish,
          language: 'TURKISH',
          chapter_title: row.chapter_title_turkish,
        },
      ]);

      processedRows += 1;
      progressBar.update(processedRows);
    }
    await prisma.verses.createMany({
      data: verseList,
    });
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
