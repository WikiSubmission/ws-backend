import { Injectable } from '@nestjs/common';
import { randomInt } from 'crypto';
import { PrismaService } from '../prisma/prisma.service';
import { Chapter, Languages, Qiraats, Verse } from '@prisma/client';
import { regex } from './regex.constant';
import { LoggerService } from '../logger/logger.service';
import { argv0 } from 'process';

@Injectable()
export class ApiRestQuranService {
  constructor(
    private readonly prisma: PrismaService,
    private readonly logger: LoggerService,
  ) {}

  public async getRandomChapter() {
    this.logger.log({
      message: `getRandomChapter`,
      context: 'ApiRestQuranController.getRandomChapter',
    });
    const chapterNum = randomInt(1, 115);
    const chapter = await this.prisma.chapter.findFirstOrThrow({
      where: {
        number: chapterNum,
        quran: {
          language: 'ENGLISH',
        },
      },
      include: {
        verses: {
          omit: {
            id: true,
            chapterId: true,
          },
        },
      },
      omit: {
        id: true,
        quranId: true,
      },
    });

    return chapter;
  }

  public async getRandomVerse(chapterFrom?: number, chapterTo?: number) {
    const chapterNum = randomInt(
      chapterFrom ? Math.max(1, chapterFrom) : 1,
      chapterTo ? Math.min(chapterTo, 115) : 115,
    );
    const chapter = await this.prisma.chapter.findFirstOrThrow({
      where: {
        number: chapterNum,
        quran: {
          language: 'ENGLISH',
        },
      },
      include: {
        verses: true,
      },
    });

    const verseCount = chapter.verses.length;
    const verseNum = randomInt(1, verseCount + 1);
    const verse = chapter.verses.find((v) => v.number === verseNum);
    return {
      chapterNum,
      verseNum,
      subtitle: verse?.subtitle,
      content: verse?.content,
      footnote: verse?.footnote,
    };
  }

  private parseQuranQuery(input: string) {
    this.logger.log({ message: `input: ${input}` });
    // if (regex.test(String(input).trim())) throw new Error('invalid entry');
    try {
      const entries = input.split(',').map((e) => e.trim());
      const parsedEntries: QuranRef[] = entries.map((entry) => {
        const [chapterPart, versePart] = entry.split(':').map((e) => e.trim());
        const chapterNum = parseInt(chapterPart);
        if (!versePart) {
          return {
            chapterNum,
            type: 'chapter',
          };
        }

        if (versePart.includes('-')) {
          const [start, end] = versePart
            .split('-')
            .map((p) => parseInt(p.trim()));
          return {
            chapterNum,
            start,
            end,
            type: 'range',
          };
        } else {
          const verse = parseInt(versePart);
          return {
            chapterNum,
            verse,
            type: 'verse',
          };
        }
      });
      return parsedEntries;
    } catch (error) {
      this.logger.error({
        context: 'ApiRestQuranService.parseQuranQuery',
        message: error instanceof Error ? error.message : 'Unknown error',
      });
      throw new Error('invalid entry');
    }
  }

  private async getChapter(
    chapterNum: number,
    language: Languages = 'ENGLISH',
    qiraat: Qiraats | null = null,
  ) {
    const chapter = await this.prisma.chapter.findFirstOrThrow({
      where: {
        number: chapterNum,
        quran: {
          language: language,
          qiraat: qiraat,
        },
      },
      include: {
        verses: {
          omit: {
            id: true,
            chapterId: true,
          },
        },
      },
      omit: {
        id: true,
        quranId: true,
      },
    });

    return {
      type: 'chapter',
      data: chapter,
    };
  }

  private async getVerse(
    chapterNum: number,
    verseNum: number,
    language: Languages = 'ENGLISH',
    qiraat: Qiraats | null = null,
  ) {
    const verse = await this.prisma.verse.findFirstOrThrow({
      where: {
        number: verseNum,
        chapter: {
          number: chapterNum,
          quran: {
            language: language,
            qiraat: qiraat,
          },
        },
      },
      omit: {
        id: true,
        chapterId: true,
      },
      include: {
        chapter: {
          omit: {
            id: true,
            quranId: true,
          },
        },
      },
    });

    return {
      type: 'verse',

      chapterNum: chapterNum,
      chapterTitle: verse.chapter.name,
      data: verse,
    };
  }

  private async getVerses(
    chapterNum: number,
    start: number,
    end: number,
    language: Languages = 'ENGLISH',
    qiraat: Qiraats | null = null,
  ) {
    const chapter = await this.prisma.chapter.findFirstOrThrow({
      where: {
        number: chapterNum,
        quran: {
          qiraat: qiraat,
          language: language,
        },
      },
    });
    const verses = await this.prisma.verse.findMany({
      where: {
        number: {
          gte: start,
          lte: end,
        },
        chapter: {
          number: chapterNum,
          quran: {
            language: language,
            qiraat: qiraat,
          },
        },
      },
      omit: {
        id: true,
        chapterId: true,
      },
    });

    return {
      type: 'verses',
      chapterNum: chapterNum,
      chapterTitle: chapter.name,
      data: verses,
    };
  }

  public async getQuran(query: string) {
    const parsedEntries = this.parseQuranQuery(query);
    const ans = [];
    for (const entry of parsedEntries) {
      if (entry.type === 'chapter') {
        const chapter = await this.getChapter(entry.chapterNum);
        ans.push(chapter);
      } else if (entry.type === 'verse') {
        const verse = await this.getVerse(entry.chapterNum, entry.verse);
        ans.push(verse);
      } else if (entry.type === 'range') {
        const verses = await this.getVerses(
          entry.chapterNum,
          entry.start,
          entry.end,
        );
        ans.push(verses);
      }
    }
    this.logger.log(ans);
    return ans;
  }
}

type QuranRef =
  | { chapterNum: number; type: 'chapter' }
  | { chapterNum: number; verse: number; type: 'verse' }
  | { chapterNum: number; start: number; end: number; type: 'range' };
type ResponseType =
  | {
      type: 'chapter';
      data: Omit<Chapter, 'id'>;
    }
  | {
      type: 'verse';
      chapterNum: number;
      chapterTitle: string;
      data: Omit<Verse, 'id'>;
    }
  | {
      type: 'verses';
      chapterNum: number;
      chapterTitle: string;
      data: Omit<Verse, 'id'>[];
    };
