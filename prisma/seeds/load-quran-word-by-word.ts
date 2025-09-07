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

    // preload all verses
    const verses = await prisma.verses.findMany({
      where: {
        language: {
          in: [
            'ARABIC_NON_TASHKEEL',
            'ARABIC_TASHKEEL',
            'ENGLISH',
            'TRANSLITERATED',
          ],
        },
      },
      select: {
        uuid: true,
        chapter_index: true,
        verse_index: true,
        language: true,
      },
    });
    const verseMap = new Map<string, string>();
    for (const v of verses) {
      verseMap.set(`${v.chapter_index}:${v.verse_index}:${v.language}`, v.uuid);
    }

    const progressBar = new cliProgress.SingleBar(
      {},
      cliProgress.Presets.shades_classic,
    );
    progressBar.start(rowCount, 0);

    let processedRows = 0;
    let WordList: Omit<Words, 'uuid'>[] = [];
    const CHUNK_SIZE = 5000;

    const parser = fs
      .createReadStream(csvFilePath)
      .pipe(parse({ columns: true }));
    for await (const untypedRow of parser) {
      const row = untypedRow as WsQuranWordByWord;
      const [chapter, verse] = row.verse_id.split(':').map(Number);

      const base = {
        global_index: Number(row.global_index),
        chapter_index: chapter,
        verse_index: verse,
        word_index: Number(row.word_index),
        root: row.root_word,
      };

      WordList.push(
        {
          ...base,
          content: row.arabic_text,
          language: 'ARABIC_NON_TASHKEEL',
          versesUuid: verseMap.get(`${chapter}:${verse}:ARABIC_NON_TASHKEEL`)!,
        },
        {
          ...base,
          content: row.arabic_text,
          language: 'ARABIC_TASHKEEL',
          versesUuid: verseMap.get(`${chapter}:${verse}:ARABIC_TASHKEEL`)!,
        },
        {
          ...base,
          content: row.english_text,
          language: 'ENGLISH',
          versesUuid: verseMap.get(`${chapter}:${verse}:ENGLISH`)!,
        },
        {
          ...base,
          content: row.transliterated_text,
          language: 'TRANSLITERATED',
          versesUuid: verseMap.get(`${chapter}:${verse}:TRANSLITERATED`)!,
        },
      );

      processedRows++;
      progressBar.update(processedRows);

      if (WordList.length >= CHUNK_SIZE) {
        await prisma.words.createMany({ data: WordList });
        WordList = [];
      }
    }

    // flush remaining
    if (WordList.length > 0) {
      await prisma.words.createMany({ data: WordList });
    }

    progressBar.stop();
    console.log(`Finished import ${processedRows * 4} words`);
    await app.close();
  } catch (error) {
    console.error('Error while importing.', error);
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
