/* eslint-disable @typescript-eslint/no-misused-promises */
import { NestFactory } from '@nestjs/core';
import { AppModule } from 'src/app.module';
import { PrismaService } from 'src/modules';
import * as fs from 'fs';
import { parse } from 'csv-parse';
import * as cliProgress from 'cli-progress';
import { Quran } from '@prisma/client';

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
    const csvFilePath =
      '/home/hichem/Projects/wikisubmission-backend-quran/data/ws-quran-foreign_rows.csv';

    const rowCount = await countCsvRows(csvFilePath);
    console.log('csv rows are: ', rowCount);
    const progressBar = new cliProgress.SingleBar(
      {},
      cliProgress.Presets.shades_classic,
    );

    // get / create the qurans

    const quran = await prisma.quran.findFirstOrThrow({
      where: {
        language: 'ENGLISH',
      },
    });

    const french = await prisma.quran.create({
      data: {
        language: 'FR_NCH',
      },
    });

    const swedish = await prisma.quran.create({
      data: {
        language: 'SWEDISH',
      },
    });

    const german = await prisma.quran.create({
      data: {
        language: 'GERMAN',
      },
    });

    const bahasa = await prisma.quran.create({
      data: {
        language: 'BAHASA',
      },
    });

    const tamil = await prisma.quran.create({
      data: {
        language: 'TAMIL',
      },
    });

    const persian = await prisma.quran.create({
      data: {
        language: 'PERSIAN',
      },
    });

    progressBar.start(rowCount, 0);
    let processedRows = 0;
    const parser = fs
      .createReadStream(csvFilePath)
      .pipe(parse({ columns: true }));
    for await (const untypedRow of parser) {
      const row = untypedRow as WsQuranForeign;
      const [chapter, verse] = row.verse_id.split(':').map(Number);

      const currentChapter = await prisma.chapter.findUniqueOrThrow({
        where: {
          quranId_number: {
            quranId: quran.id,
            number: chapter,
          },
        },
      });

      const frenchChapter = await prisma.chapter.upsert({
        where: {
          quranId_number: {
            quranId: french.id,
            number: chapter,
          },
        },
        update: {},
        create: {
          quran: {
            connect: {
              id: french.id,
            },
          },
          revelation_order: currentChapter?.revelation_order,
          number: chapter,
          name: row.chapter_title_french,
        },
      });

      const swedishChapter = await prisma.chapter.upsert({
        where: {
          quranId_number: {
            quranId: swedish.id,
            number: chapter,
          },
        },
        update: {},
        create: {
          quran: {
            connect: {
              id: swedish.id,
            },
          },
          revelation_order: currentChapter?.revelation_order,
          number: chapter,
          name: row.chapter_title_swedish,
        },
      });

      const germanChapter = await prisma.chapter.upsert({
        where: {
          quranId_number: {
            quranId: german.id,
            number: chapter,
          },
        },
        update: {},
        create: {
          quran: {
            connect: {
              id: german.id,
            },
          },
          revelation_order: currentChapter?.revelation_order,
          number: chapter,
          name: row.chapter_title_german,
        },
      });

      const bahasaChapter = await prisma.chapter.upsert({
        where: {
          quranId_number: {
            quranId: bahasa.id,
            number: chapter,
          },
        },
        update: {},
        create: {
          quran: {
            connect: {
              id: bahasa.id,
            },
          },
          revelation_order: currentChapter?.revelation_order,
          number: chapter,
          name: row.chapter_title_bahasa,
        },
      });

      const tamilChapter = await prisma.chapter.upsert({
        where: {
          quranId_number: {
            quranId: tamil.id,
            number: chapter,
          },
        },
        update: {},
        create: {
          quran: {
            connect: {
              id: tamil.id,
            },
          },
          revelation_order: currentChapter?.revelation_order,
          number: chapter,
          name: row.chapter_title_tamil,
        },
      });

      const persianChapter = await prisma.chapter.upsert({
        where: {
          quranId_number: {
            quranId: persian.id,
            number: chapter,
          },
        },
        update: {},
        create: {
          quran: {
            connect: {
              id: persian.id,
            },
          },
          revelation_order: currentChapter?.revelation_order,
          number: chapter,
          name: row.chapter_title_persian,
        },
      });

      // verses
      const verseData = await prisma.verse.findUniqueOrThrow({
        where: {
          chapterId_number: {
            chapterId: currentChapter.id,
            number: verse,
          },
        },
      });

      const frenchVerse = await prisma.verse.create({
        data: {
          chapter: {
            connect: {
              id: frenchChapter.id,
            },
          },
          number: verse,
          subtitle: row.verse_subtitle_french,
          content: row.verse_text_french,
          footnote: row.verse_footnote_french,
        },
      });

      const swedishVerse = await prisma.verse.create({
        data: {
          chapter: {
            connect: {
              id: swedishChapter.id,
            },
          },
          number: verse,
          subtitle: row.verse_subtitle_swedish,
          content: row.verse_text_swedish,
          footnote: row.verse_footnote_swedish,
        },
      });

      const persianVerse = await prisma.verse.create({
        data: {
          chapter: {
            connect: {
              id: persianChapter.id,
            },
          },
          number: verse,
          subtitle: row.verse_subtitle_persian,
          content: row.verse_text_persian,
          footnote: row.verse_footnote_persian,
        },
      });

      const tamilVerse = await prisma.verse.create({
        data: {
          chapter: {
            connect: {
              id: tamilChapter.id,
            },
          },
          number: verse,
          subtitle: row.verse_subtitle_tamil,
          content: row.verse_text_tamil,
          footnote: row.verse_footnote_tamil,
        },
      });

      const bahasaVerse = await prisma.verse.create({
        data: {
          chapter: {
            connect: {
              id: bahasaChapter.id,
            },
          },
          number: verse,
          subtitle: row.verse_subtitle_bahasa,
          content: row.verse_text_bahasa,
          footnote: row.verse_footnote_bahasa,
        },
      });

      const germanVerse = await prisma.verse.create({
        data: {
          chapter: {
            connect: {
              id: germanChapter.id,
            },
          },
          number: verse,
          subtitle: row.verse_subtitle_german,
          content: row.verse_text_german,
          footnote: row.verse_text_german,
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
bootstrapQT();
