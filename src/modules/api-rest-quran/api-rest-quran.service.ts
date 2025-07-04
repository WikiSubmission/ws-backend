import { Injectable } from '@nestjs/common';
import { randomInt } from 'crypto';
import { PrismaService } from '../prisma/prisma.service';
import {
  Chapter,
  Languages,
  Qiraats,
  Scripts,
  Verse,
  Word,
} from '@prisma/client';
import { LoggerService } from '../logger/logger.service';
import { ApiRestaudioService } from './api-restaudio/api-restaudio.service';
import { ParsedQueryList } from './types';
import {
  ParsedQueryItem,
  ValidParsedQueryItem,
  InvalidParsedQueryItem,
  EnrichedQueryItem,
  ProcessedQueryItem,
  VerseWithRelations,
} from './types/query';
import { QuranQueryParams } from './dto/quran-query.dto';

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
          type: 'invalid',
          queryWithError: '',
          errors: ['Input is empty.'],
        },
        ...(rest_parsed ?? []),
      ];
    }
    if (/^\d+$/.test(input)) {
      const chapterNum = parseInt(input);
      if (chapterNum > 0 && chapterNum <= 114) {
        return [{ type: 'valid', chapterNum }, ...(rest_parsed ?? [])];
      } else {
        return [
          {
            type: 'invalid',
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
            type: 'invalid',
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
              type: 'valid',
              chapterNum,
              startingVerseNum: startVerse,
              endingVerseNum: endVerse,
            },
            ...(rest_parsed ?? []),
          ];
        } else {
          return [
            {
              type: 'invalid',
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
              type: 'valid',
              chapterNum,
              verseNum,
            },
            ...(rest_parsed ?? []),
          ];
        } else {
          return [
            {
              type: 'invalid',
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

  private matchVerses({
    item,
    targetedVerses,
    fullChapterVerses,
  }: {
    item: ParsedQueryItem;
    targetedVerses: (Verse & {
      words: Word[];
      chapter: Chapter;
    })[];
    fullChapterVerses: (Verse & {
      words: Word[];
      chapter: Chapter;
    })[];
  }) {
    if (item.type === 'invalid') return [];

    if (item.verseNum) {
      return targetedVerses.filter(
        (v) =>
          v.chapter.number === item.chapterNum && v.number === item.verseNum,
      );
    } else if (item.startingVerseNum && item.endingVerseNum) {
      return targetedVerses.filter(
        (v) =>
          v.chapter.number === item.chapterNum &&
          v.number >= item.startingVerseNum! &&
          v.number <= item.endingVerseNum!,
      );
    } else {
      return fullChapterVerses.filter(
        (v) => v.chapter.number === item.chapterNum,
      );
    }
  }

  /**
   * Injects verse and word data (in both a main and optional secondary language)
   * for a list of parsed Quranic queries. It supports:
   * - Full chapter queries
   * - Single verse queries
   * - Verse range queries
   *
   * For each query item, it returns:
   * - The full concatenated text (`fullText`)
   * - An array of verses (`verseByVerse`) with word-by-word breakdown
   * - Optionally, parallel verse content from a secondary language
   *
   * All data is batch-fetched to minimize Prisma DB calls.
   *
   * @param request - Parsed query input list (valid or invalid entries)
   * @param mainLanguage - Primary language of Quran content (e.g., `ENGLISH`, `ARABIC_TASHKEEL`)
   * @param outputType - Output style: `wordbyword`, `versebyverse`, or `chapter`
   * @param mainQiraat - Optional Qiraat variant for main language
   * @param mainScript - Optional script style for main language
   * @param secondaryLanguage - Optional secondary language for bilingual output
   * @param secondaryQiraat - Optional Qiraat for secondary language
   * @param secondaryScript - Optional script for secondary language
   * @param includeWordByWord - Whether to include words inside each verse (default: false)
   * @param sendFullText - Whether to include concatenated full text in the result (default: false)
   * @param includeAudioVerse - Whether to add audio URLs to verses (default: false)
   * @param includeAudioChapter - Whether to add audio URLs at the chapter level (default: false)
   * @returns Enriched query results with full text, verses, and words (optionally bilingual)
   */
  private async injectLanguageData({
    request,
    mainLanguage,
    mainQiraat,
    mainScript,
    secondaryLanguage,
    secondaryQiraat,
    secondaryScript,
    includeWordByWord = false,
    sendFullText = false,
    includeAudioVerse = false,
    includeAudioChapter = false,
    bitrate = 128,
    edition = 'ar.alafasy',
  }: {
    request: ParsedQueryList;
    mainLanguage: Languages;
    outputType: 'wordbyword' | 'versebyverse' | 'chapter';
    mainQiraat?: Qiraats;
    mainScript?: Scripts;
    secondaryLanguage?: Languages;
    secondaryQiraat?: Qiraats;
    secondaryScript?: Scripts;
    sendFullText?: boolean;
    includeWordByWord?: boolean;
    includeAudioVerse?: boolean;
    includeAudioChapter?: boolean;
    bitrate?: number;
    edition?: string;
  }): Promise<ProcessedQueryItem[]> {
    this.logger.log(`parameters: mainlanguage: ${mainLanguage}`);
    this.logger.log(`parameters: includeWordByWord: ${includeWordByWord}`);
    this.logger.log(`parameters: includeAudioVerse: ${includeAudioVerse}`);
    this.logger.log(`parameters: includeAudioChapter: ${includeAudioChapter}`);

    type VerseTarget = { chapterNum: number; verseNum: number };

    const targets: VerseTarget[] = [];
    const fullChapterNums = new Set<number>();

    const t0 = performance.now();

    // Classify request items into full chapters or specific verse targets while removing invalid items
    for (const item of request) {
      if (item.type === 'invalid') continue;

      if (item.verseNum) {
        // case of request one verse
        targets.push({ chapterNum: item.chapterNum, verseNum: item.verseNum });
      } else if (item.startingVerseNum && item.endingVerseNum) {
        // case of request range of verses
        for (let v = item.startingVerseNum; v <= item.endingVerseNum; v++) {
          targets.push({ chapterNum: item.chapterNum, verseNum: v });
        }
      } else {
        // case of requesting full chapter
        fullChapterNums.add(item.chapterNum);
      }
    }

    const t1 = performance.now();

    // Build optimized include object based on what's actually needed
    const buildInclude = (includeWords: boolean) => ({
      chapter: {
        include: {
          quran: true,
        },
      },
      ...(includeWords && { words: true }),
    });

    // Build unified WHERE clause for main language
    const buildMainWhereClause = () => {
      const conditions: any[] = [];

      // Add full chapter conditions
      if (fullChapterNums.size > 0) {
        conditions.push({
          chapter: {
            number: { in: [...fullChapterNums] },
            quran: {
              language: mainLanguage,
              ...(mainQiraat && { qiraat: mainQiraat }),
              ...(mainScript && { Script: mainScript }),
            },
          },
        });
      }

      // Add targeted verse conditions
      if (targets.length > 0) {
        conditions.push(
          ...targets.map(({ chapterNum, verseNum }) => ({
            chapter: {
              number: chapterNum,
              quran: {
                language: mainLanguage,
                ...(mainQiraat && { qiraat: mainQiraat }),
                ...(mainScript && { Script: mainScript }),
              },
            },
            number: verseNum,
          })),
        );
      }

      return conditions.length > 0 ? { OR: conditions } : {};
    };

    // Build unified WHERE clause for secondary language
    const buildSecondaryWhereClause = () => {
      if (!secondaryLanguage) return {};

      const conditions: any[] = [];

      // Add full chapter conditions
      if (fullChapterNums.size > 0) {
        conditions.push({
          chapter: {
            number: { in: [...fullChapterNums] },
            quran: {
              language: secondaryLanguage,
              ...(secondaryQiraat && { qiraat: secondaryQiraat }),
              ...(secondaryScript && { Script: secondaryScript }),
            },
          },
        });
      }

      // Add targeted verse conditions
      if (targets.length > 0) {
        conditions.push(
          ...targets.map(({ chapterNum, verseNum }) => ({
            chapter: {
              number: chapterNum,
              quran: {
                language: secondaryLanguage,
                ...(secondaryQiraat && { qiraat: secondaryQiraat }),
                ...(secondaryScript && { Script: secondaryScript }),
              },
            },
            number: verseNum,
          })),
        );
      }

      return conditions.length > 0 ? { OR: conditions } : {};
    };

    const t2 = performance.now();

    // Execute queries in parallel instead of transaction for better performance
    const queryPromises: Promise<VerseWithRelations[]>[] = [];

    // Main language query
    const mainWhereClause = buildMainWhereClause();
    if (Object.keys(mainWhereClause).length > 0) {
      queryPromises.push(
        this.prisma.verse.findMany({
          where: mainWhereClause,
          include: buildInclude(includeWordByWord),
          orderBy: [{ chapter: { number: 'asc' } }, { number: 'asc' }],
        }) as Promise<VerseWithRelations[]>,
      );
    } else {
      queryPromises.push(Promise.resolve([]));
    }

    // Secondary language query (only if needed)
    if (secondaryLanguage) {
      const secondaryWhereClause = buildSecondaryWhereClause();
      if (Object.keys(secondaryWhereClause).length > 0) {
        queryPromises.push(
          this.prisma.verse.findMany({
            where: secondaryWhereClause,
            include: buildInclude(false), // Secondary language doesn't need words
            orderBy: [{ chapter: { number: 'asc' } }, { number: 'asc' }],
          }) as Promise<VerseWithRelations[]>,
        );
      } else {
        queryPromises.push(Promise.resolve([]));
      }
    }

    const t3 = performance.now();

    // Execute all queries in parallel
    const results = await Promise.all(queryPromises);
    const [allMainVerses, allSecondaryVerses = []] = results;

    const t4 = performance.now();

    // Build a lookup for (chapter:verse) => secondaryContent
    const secondaryVerseMap = new Map<string, string>();
    if (secondaryLanguage && allSecondaryVerses.length > 0) {
      for (const v of allSecondaryVerses) {
        const key = `${v.chapter.number}:${v.number}`;
        secondaryVerseMap.set(key, v.content);
      }
    }

    const t5 = performance.now();

    // Build response for each input item
    const result: ProcessedQueryItem[] = request.map((item) => {
      if (item.type === 'invalid') return item;

      const verses = this.matchVersesOptimized({
        item,
        allVerses: allMainVerses,
      });

      const enrichedItem: EnrichedQueryItem = {
        ...item,
        data: {
          type: 'verses',
          ...(sendFullText && {
            fullText: verses.map((v) => v.content).join(' '),
          }),
          ...(includeAudioChapter && {
            audio: this.audio.buildChapterAudioUrl({
              bitrate: bitrate,
              edition: edition,
              number: item.chapterNum,
            }),
          }),
          verseByVerse: verses.map((v) => {
            const key = `${v.chapter.number}:${v.number}`;
            return {
              verse: {
                number: v.number,
                content: v.content,
                ...(secondaryLanguage && {
                  secondaryContent: secondaryVerseMap.get(key) ?? null,
                }),
                subtitle: v.subtitle,
                footnote: v.footnote,
                ...(includeAudioVerse && {
                  audio: this.audio.buildVerseByVerseAudioUrl({
                    bitrate: bitrate,
                    edition: edition,
                    number: `${v.chapter.number}${v.number.toString().padStart(3, '0')}`,
                  }),
                }),
              },
              ...(includeWordByWord &&
                v.words && {
                  words: v.words.map((w) => ({
                    number: w.number,
                    content: w.content,
                    root: w.root,
                  })),
                }),
            };
          }),
        },
      };

      return enrichedItem;
    });

    const t6 = performance.now();

    // Optimized logging
    this.logger.log(`Phase 1 (Classification): ${t1 - t0}ms`);
    this.logger.log(`Phase 2 (Query Building): ${t2 - t1}ms`);
    this.logger.log(`Phase 3 (Query Preparation): ${t3 - t2}ms`);
    this.logger.log(`Phase 4 (Database Execution): ${t4 - t3}ms`);
    this.logger.log(`Phase 5 (Secondary Map Building): ${t5 - t4}ms`);
    this.logger.log(`Phase 6 (Result Building): ${t6 - t5}ms`);

    return result;
  }

  /**
   * Optimized verse matching that works with unified verse data
   */
  private matchVersesOptimized({
    item,
    allVerses,
  }: {
    item: ValidParsedQueryItem;
    allVerses: VerseWithRelations[];
  }): VerseWithRelations[] {
    const chapterNum = item.chapterNum;

    if (item.verseNum) {
      // Single verse
      return allVerses.filter(
        (v) => v.chapter.number === chapterNum && v.number === item.verseNum,
      );
    } else if (item.startingVerseNum && item.endingVerseNum) {
      // Verse range
      return allVerses.filter(
        (v) =>
          v.chapter.number === chapterNum &&
          v.number >= item.startingVerseNum! &&
          v.number <= item.endingVerseNum!,
      );
    } else {
      // Full chapter
      return allVerses.filter((v) => v.chapter.number === chapterNum);
    }
  }

  public getQuranFromQuery({ query }: { query: QuranQueryParams }) {
    const t0 = performance.now();
    // Convert string query to machine readable list of Quran references
    const parsedRequest = this.parseQuranQuery(query.query);

    const t1 = performance.now();

    // inject language
    const withMainLanguage = this.injectLanguageData({
      request: parsedRequest,
      mainLanguage: query.mainLanguage,
      mainQiraat: query.mainQiraat,
      mainScript: query.mainScript,
      secondaryLanguage: query.secondaryLanguage,
      secondaryQiraat: query.secondaryQiraat,
      secondaryScript: query.secondaryScript,
      includeAudioChapter: query.includeChapterAudio,
      includeAudioVerse: query.includeVerseByVerseAudio,
      bitrate: query.audioBitrate,
      edition: query.audioIdentifier,
      outputType: query.mainType,
      includeWordByWord: query.includeWordByWord,
      sendFullText: query.sendFullText,
    });
    const t2 = performance.now();
    this.logger.log(`parsing: ${t1 - t0}ms`);
    this.logger.log(`injecting language: ${t2 - t1}ms`);

    const res = withMainLanguage;
    return res;
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
