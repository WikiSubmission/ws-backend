import { NestFactory } from '@nestjs/core';
import { AppModule } from '../../src/app.module';
import { PrismaService } from '@modules';
import * as fs from 'fs';
import { parse } from 'csv-parse';
import * as cliProgress from 'cli-progress';
import { Words } from '@prisma/client';

export type WsQuranWordByWord = {
  global_index: number;
  word_index: number;
  verse_id: string;
  english_text: string;
  transliterated_text: string;
  arabic_text: string;
  root_word: string;
};

export async function bootstrapWBW() {
  const app = await NestFactory.createApplicationContext(AppModule);
  const prisma = app.get(PrismaService);

  try {
    await prisma.words.deleteMany({});
    console.log('Starting Quran words CSV import...');
    const csvFilePath = 'prisma/seeds/data/ws-quran-word-by-word_rows.csv';

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

    let WordList: Omit<Words, 'uuid'>[] = [];
    for await (const untypedRow of parser) {
      let row = untypedRow as WsQuranWordByWord;
      row = {
        ...row,
        word_index: Number(row.word_index),
        global_index: Number(row.global_index),
      };
      const [chapter, verse] = row.verse_id.split(':').map(Number);
      const ARABIC_NON_TASHKEEL_verse = await prisma.verses.findFirstOrThrow({
        where: {
          chapter_index: chapter,
          verse_index: verse,
          language: 'ARABIC_NON_TASHKEEL',
        },
      });

      const ARABIC_TASHKEEL_verse = await prisma.verses.findFirstOrThrow({
        where: {
          chapter_index: chapter,
          verse_index: verse,
          language: 'ARABIC_TASHKEEL',
        },
      });

      const ENGLISH_verse = await prisma.verses.findFirstOrThrow({
        where: {
          chapter_index: chapter,
          verse_index: verse,
          language: 'ENGLISH',
        },
      });

      const TRANSLITERATED_verse = await prisma.verses.findFirstOrThrow({
        where: {
          chapter_index: chapter,
          verse_index: verse,
          language: 'TRANSLITERATED',
        },
      });

      WordList = WordList.concat([
        {
          global_index: row.global_index,
          chapter_index: chapter,
          verse_index: verse,
          word_index: row.word_index,
          root: row.root_word,
          content: row.arabic_text,
          language: 'ARABIC_NON_TASHKEEL',
          versesUuid: ARABIC_NON_TASHKEEL_verse.uuid,
        },
        {
          global_index: row.global_index,
          chapter_index: chapter,
          verse_index: verse,
          word_index: row.word_index,
          root: row.root_word,
          content: row.arabic_text,
          language: 'ARABIC_TASHKEEL',
          versesUuid: ARABIC_TASHKEEL_verse.uuid,
        },
        {
          global_index: row.global_index,
          chapter_index: chapter,
          verse_index: verse,
          word_index: row.word_index,
          root: row.root_word,
          content: row.english_text,
          language: 'ENGLISH',
          versesUuid: ENGLISH_verse.uuid,
        },
        {
          global_index: row.global_index,
          chapter_index: chapter,
          verse_index: verse,
          word_index: row.word_index,
          root: row.root_word,
          content: row.transliterated_text,
          language: 'TRANSLITERATED',
          versesUuid: TRANSLITERATED_verse.uuid,
        },
      ]);

      processedRows += 1;
      progressBar.update(processedRows);
    }
    try {
      await prisma.words.createMany({
        data: WordList,
      });
    } catch (err) {
      console.error('Failed to insert words', err);
    }
    progressBar.stop();
    console.log(`Finished import ${WordList.length} words`);
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
