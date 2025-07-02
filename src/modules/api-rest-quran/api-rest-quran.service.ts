import { Injectable } from '@nestjs/common';
import { randomInt } from 'crypto';
import { PrismaService } from '../prisma/prisma.service';
import { Chapter, Languages, Qiraats, Verse } from '@prisma/client';
import { LoggerService } from '../logger/logger.service';
import { ApiRestaudioService } from './api-restaudio/api-restaudio.service';
import { ParsedQueryList } from './types';

@Injectable()
export class ApiRestQuranService {
  constructor(
    private readonly prisma: PrismaService,
    private readonly logger: LoggerService,
    private readonly audio: ApiRestaudioService,
  ) {}

  private readonly verseCounts: Record<number, number> = {
    1: 7,
    2: 286,
    3: 200,
    4: 176,
    5: 120,
    6: 165,
    7: 206,
    8: 75,
    9: 127,
    10: 109,
    11: 123,
    12: 111,
    13: 43,
    14: 52,
    15: 99,
    16: 128,
    17: 111,
    18: 110,
    19: 98,
    20: 135,
    21: 112,
    22: 78,
    23: 118,
    24: 64,
    25: 77,
    26: 227,
    27: 93,
    28: 88,
    29: 69,
    30: 60,
    31: 34,
    32: 30,
    33: 73,
    34: 54,
    35: 45,
    36: 83,
    37: 182,
    38: 88,
    39: 75,
    40: 85,
    41: 54,
    42: 53,
    43: 89,
    44: 59,
    45: 37,
    46: 35,
    47: 38,
    48: 29,
    49: 18,
    50: 45,
    51: 60,
    52: 49,
    53: 62,
    54: 55,
    55: 78,
    56: 96,
    57: 29,
    58: 22,
    59: 24,
    60: 13,
    61: 14,
    62: 11,
    63: 11,
    64: 18,
    65: 12,
    66: 12,
    67: 30,
    68: 52,
    69: 52,
    70: 44,
    71: 28,
    72: 28,
    73: 20,
    74: 56,
    75: 40,
    76: 31,
    77: 50,
    78: 40,
    79: 46,
    80: 42,
    81: 29,
    82: 19,
    83: 36,
    84: 25,
    85: 22,
    86: 17,
    87: 19,
    88: 26,
    89: 30,
    90: 20,
    91: 15,
    92: 21,
    93: 11,
    94: 8,
    95: 8,
    96: 19,
    97: 5,
    98: 8,
    99: 8,
    100: 11,
    101: 11,
    102: 8,
    103: 3,
    104: 9,
    105: 5,
    106: 4,
    107: 7,
    108: 3,
    109: 6,
    110: 3,
    111: 5,
    112: 4,
    113: 5,
    114: 6,
  };

  private getTraditionalVerseIndex({
    chapterNumber,
    verseNumber,
  }: {
    chapterNumber: number;
    verseNumber: number;
  }): number {
    if (!this.verseCounts[chapterNumber]) {
      throw new Error(`Invalid chapter number: ${chapterNumber}`);
    }
    if (chapterNumber === 9 && (verseNumber === 128 || verseNumber === 129)) {
      throw new Error('Invalid verse number: 9:128 or 9:129');
    }
    if (verseNumber > this.verseCounts[chapterNumber]) {
      if (
        !(chapterNumber === 9 && (verseNumber === 128 || verseNumber === 129))
      )
        throw new Error(
          `Invalid verse number: ${chapterNumber}:${verseNumber}`,
        );
    }

    let index = 0;
    for (let i = 0; i < chapterNumber; i++) {
      index += this.verseCounts[i];
      if (i === 9) {
        index += 2;
      }
    }

    index += verseNumber;

    return index;
  }

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

  /**
   * Parses a Quran query string (e.g., "2:3-5,3:1") into a structured format.
   *
   * Supported formats:
   * - "2" → chapter only
   * - "2:3" → single verse
   * - "2:3-5" → verse range
   * - Comma-separated values like "2,3:1,4:2-4"
   *
   * @param {string} input - The Quran query string to parse.
   * @returns {ParsedQueryList} An array of parsed query objects or error descriptions.
   */
  private parseQuranQuery(input: string): ParsedQueryList {
    let rest_parsed = undefined;
    if (input.includes(',')) {
      const [first, rest] = input.split(',');
      rest_parsed = this.parseQuranQuery(rest);
      input = first;
    }
    if (input.length === 0) {
      return [
        {
          queryWithError: '',
          errors: ['Input is empty.'],
        },
        ...(rest_parsed ?? []),
      ];
    }
    if (/^\d+$/.test(input)) {
      const chapterNum = parseInt(input);
      if (chapterNum > 0 && chapterNum <= 114) {
        return [{ chapterNum }, ...(rest_parsed ?? [])];
      } else {
        return [
          {
            queryWithError: input,
            errors: [`Chapter ${chapterNum} is not within the (1, 114) range.`],
          },
          ...(rest_parsed ?? []),
        ];
      }
    } else {
      const [chapterPart, versePart] = input.split(':').map((e) => e.trim());
      if (!chapterPart || !versePart) {
        return [
          {
            queryWithError: input,
            errors: [
              `Either the chapter part [${chapterPart}] or verse part [${versePart}] is missing.`,
            ],
          },
          ...(rest_parsed ?? []),
        ];
      }
      const chapterNum = parseInt(chapterPart);
      if (versePart.includes('-')) {
        const [startVerseStr, endVerseStr] = versePart.split('-');
        const startVerse = parseInt(startVerseStr);
        let endVerse = startVerse;
        if (endVerseStr === '') {
          endVerse = this.verseCounts[chapterNum];
        } else {
          endVerse = parseInt(endVerseStr);
        }
        if (
          startVerse > 0 &&
          startVerse <= this.verseCounts[chapterNum] &&
          endVerse > 0 &&
          endVerse <= this.verseCounts[chapterNum] &&
          startVerse <= endVerse
        ) {
          return [
            {
              chapterNum,
              startingVerseNum: startVerse,
              endingVerseNum: endVerse,
            },
            ...(rest_parsed ?? []),
          ];
        } else {
          return [
            {
              queryWithError: input,
              errors: [
                `Either the chapter part [${chapterPart}] is not within the [1, 114] range or verse part [${startVerse}, ${endVerse}] is not within the corresponding verse range for this chapter[${[1, 9].includes(chapterNum) ? '1' : '0'},${this.verseCounts[chapterNum]}].`,
              ],
            },
            ...(rest_parsed ?? []),
          ];
        }
      } else {
        const verseNum = parseInt(versePart);
        if (
          chapterNum > 0 &&
          chapterNum <= 114 &&
          verseNum > 0 &&
          verseNum <= this.verseCounts[chapterNum]
        ) {
          return [
            {
              chapterNum,
              verseNum,
            },
            ...(rest_parsed ?? []),
          ];
        } else {
          return [
            {
              queryWithError: input,
              errors: [
                `Either the chapter part [${chapterPart}] is not within the [1, 114] range or verse part [${verseNum}] is not within the corresponding verse range for this chapter[${[1, 9].includes(chapterNum) ? '1' : '0'},${this.verseCounts[chapterNum]}].`,
              ],
            },
            ...(rest_parsed ?? []),
          ];
        }
      }
    }
  }

  public __parseQuranQuery(input: string) {
    this.logger.log({ message: `input: ${input}` });
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
        context: 'ApiRestQuranService.__parseQuranQuery',
        message: error instanceof Error ? error.message : 'Unknown error',
      });
      throw new Error('invalid entry');
    }
  }

  private async getChapter({
    chapterNum,
    language = 'ENGLISH',
    qiraat = null,
    audio = false,
    bitrate = 128,
    edition = 'quran-simple',
  }: {
    chapterNum: number;
    language?: Languages;
    qiraat?: Qiraats | null;
    audio?: boolean;
    bitrate?: number;
    edition?: string;
  }) {
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

    if (audio) {
      const url = this.audio.buildChapterAudioUrl({
        bitrate,
        edition,
        number: chapterNum,
      });
      return {
        type: 'versebyverse',
        audio: url,
        data: {
          ...chapter,
          verses: chapter.verses.map((verse) => ({
            ...verse,
            audio: this.audio.buildVerseByVerseAudioUrl({
              bitrate: bitrate,
              edition: edition,
              number: this.getTraditionalVerseIndex({
                chapterNumber: chapterNum,
                verseNumber: verse.number,
              }),
            }),
          })),
        },
      };
    }

    return {
      type: 'chapter',
      data: chapter,
    };
  }

  private async getVerse({
    chapterNum,
    verseNum,
    language = 'ENGLISH',
    qiraat = null,
    audio = false,
    bitrate = 128,
    edition = 'quran-simple',
  }: {
    chapterNum: number;
    verseNum: number;
    language?: Languages;
    qiraat?: Qiraats | null;
    audio?: boolean;
    bitrate?: number;
    edition?: string;
  }) {
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

    if (audio) {
      const audioUrl = this.audio.buildVerseByVerseAudioUrl({
        bitrate: bitrate,
        edition: edition,
        number: this.getTraditionalVerseIndex({
          chapterNumber: chapterNum,
          verseNumber: verseNum,
        }),
      });
      return {
        type: 'verse',

        chapterNum: chapterNum,
        chapterTitle: verse.chapter.name,
        data: { ...verse, audio: audioUrl },
      };
    }

    return {
      type: 'verse',

      chapterNum: chapterNum,
      chapterTitle: verse.chapter.name,
      data: verse,
    };
  }

  private async getVerses({
    chapterNum,
    start,
    end,
    language = 'ENGLISH',
    audio = false,
    qiraat,
    bitrate = 128,
    edition = 'quran-simple',
  }: {
    chapterNum: number;
    start: number;
    end: number;
    language: Languages;
    audio?: boolean;
    qiraat?: Qiraats | null;
    bitrate?: number;
    edition?: string;
  }) {
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

    if (audio) {
      const chapterAudioUrl = this.audio.buildChapterAudioUrl({
        bitrate,
        edition,
        number: chapterNum,
      });

      return {
        type: 'verses',
        chapterNum: chapterNum,
        chapterTitle: chapter.name,
        audio: chapterAudioUrl,
        data: verses.map((verse) => {
          const audioUrl = this.audio.buildVerseByVerseAudioUrl({
            bitrate,
            edition,
            number: this.getTraditionalVerseIndex({
              chapterNumber: chapterNum,
              verseNumber: verse.number,
            }),
          });
          return { ...verse, audio: audioUrl };
        }),
      };
    }

    return {
      type: 'verses',
      chapterNum: chapterNum,
      chapterTitle: chapter.name,
      data: verses,
    };
  }

  public async getQuranFromQuery({
    query,
    language = 'ENGLISH',
    qiraat,
    audio,
  }: {
    query: string;
    language?: Languages;
    qiraat?: Qiraats;
    audio?: boolean;
  }) {
    const parsedEntries = this.__parseQuranQuery(query);
    const ans = [];
    for (const entry of parsedEntries) {
      if (entry.type === 'chapter') {
        const chapter = await this.getChapter({
          chapterNum: entry.chapterNum,
          audio: audio,
          language: language,
          qiraat: qiraat,
        });
        ans.push(chapter);
      } else if (entry.type === 'verse') {
        const verse = await this.getVerse({
          chapterNum: entry.chapterNum,
          verseNum: entry.verse,
        });
        ans.push(verse);
      } else if (entry.type === 'range') {
        const verses = await this.getVerses({
          chapterNum: entry.chapterNum,
          start: entry.start,
          end: entry.end,
          audio: audio,
          language: language,
          qiraat: qiraat,
        });
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
