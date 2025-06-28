import { NestFactory } from '@nestjs/core';
import { AppModule } from 'src/app.module';
import { PrismaService } from 'src/modules';
import * as fs from 'fs';
import { parse } from 'csv-parse';
import * as cliProgress from 'cli-progress';

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
    console.log('Starting Quran words CSV import...');
    const csvFilePath = 'prisma/seeds/data/ws-quran-word-by-word_rows.csv';

    const rowCount = await countCsvRows(csvFilePath);
    console.log('csv rows are: ', rowCount);
    const progressBar = new cliProgress.SingleBar(
      {},
      cliProgress.Presets.shades_classic,
    );

    const englishQuran = await prisma.quran.findFirstOrThrow({
      where: {
        language: 'ENGLISH',
      },
    });

    const arabicTashkeelQuran = await prisma.quran.findFirstOrThrow({
      where: {
        language: 'ARABIC_TASHKEEL',
      },
    });

    const arabicNoTashkeelQuran = await prisma.quran.findFirstOrThrow({
      where: {
        language: 'ARABIC_NON_TASHKEEL',
      },
    });

    const transliterationQuran = await prisma.quran.findFirstOrThrow({
      where: {
        language: 'TRANSLITERATED',
      },
    });

    progressBar.start(rowCount, 0);
    let processedRows = 0;
    const parser = fs
      .createReadStream(csvFilePath)
      .pipe(parse({ columns: true }));
    for await (const untypedRow of parser) {
      let row = untypedRow as WsQuranWordByWord;
      row = {
        ...row,
        word_index: Number(row.word_index),
        global_index: Number(row.global_index),
      };
      const [chapter, verse] = row.verse_id.split(':').map(Number);

      const englishChapter = await prisma.chapter.findUniqueOrThrow({
        where: {
          quranId_number: {
            quranId: englishQuran.id,
            number: chapter,
          },
        },
      });

      const arabicTashkeelChapter = await prisma.chapter.findUniqueOrThrow({
        where: {
          quranId_number: {
            quranId: arabicTashkeelQuran.id,
            number: chapter,
          },
        },
      });

      const arabicNoTashkeelChapter = await prisma.chapter.findUniqueOrThrow({
        where: {
          quranId_number: {
            quranId: arabicNoTashkeelQuran.id,
            number: chapter,
          },
        },
      });

      const transliterationChapter = await prisma.chapter.findUniqueOrThrow({
        where: {
          quranId_number: {
            quranId: transliterationQuran.id,
            number: chapter,
          },
        },
      });

      // verses
      const englishVerse = await prisma.verse.findUniqueOrThrow({
        where: {
          chapterId_number: {
            chapterId: englishChapter.id,
            number: verse,
          },
        },
      });

      const arabicTashkeelVerse = await prisma.verse.findUniqueOrThrow({
        where: {
          chapterId_number: {
            chapterId: arabicTashkeelChapter.id,
            number: verse,
          },
        },
      });

      const arabicNoTashkeelVerse = await prisma.verse.findUniqueOrThrow({
        where: {
          chapterId_number: {
            chapterId: arabicNoTashkeelChapter.id,
            number: verse,
          },
        },
      });

      const transliterationVerse = await prisma.verse.findUniqueOrThrow({
        where: {
          chapterId_number: {
            chapterId: transliterationChapter.id,
            number: verse,
          },
        },
      });

      // words
      const englishWord = await prisma.word.create({
        data: {
          verse: {
            connect: {
              id: englishVerse.id,
            },
          },
          number: row.word_index,
          content: row.english_text,
          quran_index: row.global_index,
        },
      });

      const arabicTashkeelWord = await prisma.word.create({
        data: {
          verse: {
            connect: {
              id: arabicTashkeelVerse.id,
            },
          },
          number: row.word_index,
          content: row.arabic_text,
          root: row.root_word,
          quran_index: row.global_index,
        },
      });

      const arabicNoTashkeelWord = await prisma.word.create({
        data: {
          verse: {
            connect: {
              id: arabicNoTashkeelVerse.id,
            },
          },
          number: row.word_index,
          content: row.arabic_text,
          root: row.root_word,
          quran_index: row.global_index,
        },
      });

      const transliterationWord = await prisma.word.create({
        data: {
          verse: {
            connect: {
              id: transliterationVerse.id,
            },
          },
          number: row.word_index,
          content: row.transliterated_text,
          root: row.root_word,
          quran_index: row.global_index,
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
