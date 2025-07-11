import { Injectable } from '@nestjs/common';
import { randomInt } from 'crypto';
import { PrismaService } from '../prisma/prisma.service';
import {
  Verses, // Chapter,
  // Verse,
  // Word,
} from '@prisma/client';
import { LoggerService } from '../logger/logger.service';
import { ApiRestaudioService } from './api-restaudio/api-restaudio.service';
import { ChapterResponseType, ParsedQueryList, VerseWithWords } from './types';

import {
  QuranFetchPreferences,
  QuranFetchPreferencesDto,
} from './dto/quran-query.dto';
import seedrandom from 'seedrandom';

@Injectable()
export class ApiRestQuranService {
  constructor(
    private readonly prisma: PrismaService,
    private readonly logger: LoggerService,
    private readonly audio: ApiRestaudioService,
  ) {}

  public async getRandomChapter({
    preferences,
    date,
  }: {
    preferences: QuranFetchPreferences;
    date?: Date;
  }) {
    let chapter_index: number;
    if (date) {
      const seed: string = date.toISOString().slice(0, 10);
      const rng: () => number = seedrandom(seed);
      chapter_index = Math.floor(rng() * 114) + 1;
    } else {
      chapter_index = randomInt(1, 115);
    }
    return this.getChapter({
      index: chapter_index,
      preferences: preferences,
    });
  }

  public async getRandomVerse({
    preferences,
    date,
  }: {
    preferences: QuranFetchPreferences;
    date?: Date;
  }): Promise<VerseWithWords> {
    let chapter_index: number;
    let verse_index: number;
    if (date) {
      const seed: string = date.toISOString().slice(0, 10);
      const rng: () => number = seedrandom(seed);
      chapter_index = Math.floor(rng() * 114) + 1;
      verse_index = Math.floor(rng() * this.verseCounts[chapter_index]) + 1;
    } else {
      chapter_index = randomInt(1, 115);
      verse_index = randomInt(1, this.verseCounts[chapter_index]);
    }
    const verse = await this.prisma.verses.findFirstOrThrow({
      where: {
        chapter_index: chapter_index,
        verse_index: verse_index,
        ...(preferences.includeSecondaryLanguage
          ? {
              OR: [
                { language: preferences.mainLanguage },
                { language: preferences.secondaryLanguage },
              ],
            }
          : {
              language: preferences.mainLanguage,
            }),
      },
      select: {
        global_index: true,
        chapter_index: true,
        verse_index: true,
        subtitle: true,
        content: true,
        footer: true,
        language: true,
        Words: {
          select: {
            chapter_index: true,
            verse_index: true,
            word_index: true,
            content: true,
            root: true,
          },
        },
      },
    });
    return verse;
  }

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
    const results: ParsedQueryList = [];
    const parts = input.replace(/\s+/g, '').split(',');
    const regex = /^(\d+)(?::(\d+)(?:-(\d*))?)?$/;

    for (const part of parts) {
      if (!part) {
        results.push({
          type: 'invalid',
          queryWithError: part,
          errors: ['Empty query part.'],
        });
        continue;
      }

      const match = part.match(regex);
      if (!match) {
        results.push({
          type: 'invalid',
          queryWithError: part,
          errors: ['Invalid format. Use e.g. 2, 2:3, or 2:3-5'],
        });
        continue;
      }

      const chapterNum = parseInt(match[1]);
      const verseNum = match[2] ? parseInt(match[2]) : undefined;
      const endVerseStr = match[3];

      if (!(chapterNum >= 1 && chapterNum <= 114)) {
        results.push({
          type: 'invalid',
          queryWithError: part,
          errors: [`Chapter ${chapterNum} is out of range (1–114).`],
        });
        continue;
      }

      const maxVerse = this.verseCounts[chapterNum];

      if (!verseNum) {
        results.push({ type: 'valid', chapterNum });
      } else if (!endVerseStr) {
        if (verseNum <= maxVerse) {
          results.push({ type: 'valid', chapterNum, verseNum });
        } else {
          results.push({
            type: 'invalid',
            queryWithError: part,
            errors: [
              `Verse ${verseNum} out of range for chapter ${chapterNum}.`,
            ],
          });
        }
      } else {
        const endVerse = endVerseStr === '' ? maxVerse : parseInt(endVerseStr);
        if (
          verseNum <= endVerse &&
          verseNum >= 1 &&
          endVerse >= 1 &&
          endVerse <= maxVerse
        ) {
          results.push({
            type: 'valid',
            chapterNum,
            startingVerseNum: verseNum,
            endingVerseNum: endVerse,
          });
        } else {
          results.push({
            type: 'invalid',
            queryWithError: part,
            errors: [
              `Invalid range ${verseNum}-${endVerse} for chapter ${chapterNum}.`,
            ],
          });
        }
      }
    }

    return results;
  }

  private async injectVerseData({
    parsedRequest,
    preferences,
  }: {
    parsedRequest: ParsedQueryList;
    preferences: QuranFetchPreferences;
  }): Promise<VerseWithWords[]> {
    const includeWords =
      preferences.mainLanguageIncludeWordByWord ||
      preferences.mainLanguageIncludeRootWord;

    const inputTuples: string[] = [];
    parsedRequest.forEach((item, idx) => {
      if (item.type === 'valid') {
        if (item.verseNum !== undefined) {
          inputTuples.push(
            `(${idx}, ${item.chapterNum}, ${item.verseNum}, NULL, NULL)`,
          );
        } else if (
          item.startingVerseNum !== undefined &&
          item.endingVerseNum !== undefined
        ) {
          inputTuples.push(
            `(${idx}, ${item.chapterNum}, NULL, ${item.startingVerseNum}, ${item.endingVerseNum})`,
          );
        } else {
          inputTuples.push(`(${idx}, ${item.chapterNum}, NULL, NULL, NULL)`);
        }
      }
    });

    const inputCTE = `
      WITH input_order (idx, chapter, verse, start, "end") AS (
        SELECT
          CAST(t.idx AS INT),
          CAST(t.chapter AS INT),
          CAST(t.verse AS INT),
          CAST(t.start AS INT),
          CAST(t."end" AS INT)
        FROM (
          VALUES
            ${inputTuples.join(',\n')}
        ) AS t(idx, chapter, verse, start, "end")
      )
    `;

    const languageFilter = preferences.includeSecondaryLanguage
      ? `v.language IN ('${preferences.mainLanguage}', '${preferences.secondaryLanguage}')`
      : `v.language = '${preferences.mainLanguage}'`;

    const joinWordsSQL = includeWords
      ? `
        , COALESCE(
            json_agg(
              json_build_object(
                'verse_index', w.verse_index,
                'word_index', w.word_index
                ${preferences.mainLanguageIncludeWordByWord ? `, 'content', w.content` : ''}
                ${preferences.mainLanguageIncludeRootWord ? `, 'root', w.root` : ''}
              )
              ORDER BY w.word_index
            ) FILTER (WHERE w.word_index IS NOT NULL),
            '[]'
          ) AS words
      `
      : '';

    const wordJoinSQL = includeWords
      ? `LEFT JOIN "Words" w ON w."versesUuid" = v.uuid AND w.language = '${preferences.mainLanguage}'`
      : '';

    const sql = `
      ${inputCTE}
      SELECT
        io.idx,
        v.global_index,
        v.verse_index,
        v.subtitle,
        v.content,
        v.footer,
        v.language,
        v.chapter_index,
        v.chapter_title
        ${joinWordsSQL}
      FROM input_order io
      JOIN "Verses" v
        ON v.chapter_index = io.chapter
        AND (
          (io.verse IS NOT NULL AND v.verse_index = io.verse)
          OR (io.start IS NOT NULL AND io."end" IS NOT NULL AND v.verse_index BETWEEN io.start AND io."end")
          OR (io.verse IS NULL AND io.start IS NULL AND io."end" IS NULL)
        )
        AND ${languageFilter}
      ${wordJoinSQL}
      GROUP BY io.idx, v.global_index, v.verse_index, v.subtitle, v.content, v.footer, v.language, v.chapter_index, v.chapter_title
      ORDER BY io.idx, v.verse_index,
        CASE
          WHEN v.language = '${preferences.mainLanguage}' THEN 0
          ${
            preferences.includeSecondaryLanguage
              ? `WHEN v.language = '${preferences.secondaryLanguage}' THEN 1`
              : ''
          }
          ELSE 2
        END
    `;

    return this.prisma.$queryRawUnsafe<VerseWithWords[]>(sql);
  }

  public async getChapter({
    index,
    preferences,
  }: {
    index: number;
    preferences: QuranFetchPreferences;
  }): Promise<ChapterResponseType> {
    // SQL does something like this, i'm using it over this because i can't order by language (main > secondary) in prisma orm
    // const __verses = await this.prisma.verses.findMany({
    //   where: {
    //     chapter_index: index,
    //     ...(preferences.includeSecondaryLanguage
    //       ? {
    //           OR: [
    //             { language: preferences.mainLanguage },
    //             { language: preferences.secondaryLanguage },
    //           ],
    //         }
    //       : {
    //           language: preferences.mainLanguage,
    //         }),
    //   },
    //   select: {
    //     global_index: true,
    //     verse_index: true,
    //     subtitle: true,
    //     content: true,
    //     footer: true,
    //     language: true,

    //     chapter_index: false,
    //     chapter_title: false,
    //     uuid: false,
    //     ...((preferences.mainLanguageIncludeWordByWord ||
    //       preferences.mainLanguageIncludeRootWord) && {
    //       Words: {
    //         select: {
    //           global_index: true,
    //           verse_index: true,
    //           word_index: true,
    //           content: preferences.mainLanguageIncludeWordByWord,
    //           root: preferences.mainLanguageIncludeRootWord,
    //         },
    //         orderBy: [{ verse_index: 'asc' }, { word_index: 'asc' }],
    //       },
    //     }),
    //   },
    //   orderBy: {
    //     ...(preferences.includeSecondaryLanguage
    //       ? [{ verse_index: 'asc' }, { language: 'asc' }]
    //       : { verse_index: 'asc' }),
    //   },
    // });

    const includeWords =
      preferences.mainLanguageIncludeWordByWord ||
      preferences.mainLanguageIncludeRootWord;

    const baseSelect = `
      v.global_index,
      v.verse_index,
      v.subtitle,
      v.content,
      v.footer,
      v.language
    `;

    const wordSelect = `
      , COALESCE(
          json_agg(
            json_build_object(
              'verse_index', w.verse_index,
              'word_index', w.word_index
              ${preferences.mainLanguageIncludeWordByWord ? `, 'content', w.content` : ''}
              ${preferences.mainLanguageIncludeRootWord ? `, 'root', w.root` : ''}
            )
            ORDER BY w.word_index
          ) FILTER (WHERE w.word_index IS NOT NULL),
          '[]'
        ) AS words
    `;

    const query = `
      SELECT
        ${baseSelect}
        ${includeWords ? wordSelect : ''}
      FROM "Verses" v
      ${includeWords ? `LEFT JOIN "Words" w ON v.uuid = w."versesUuid" AND w.language = '${preferences.mainLanguage}'` : ''}
      WHERE
        v.chapter_index = ${index}
        ${
          preferences.includeSecondaryLanguage
            ? `AND v.language IN ('${preferences.mainLanguage}', '${preferences.secondaryLanguage}')`
            : `AND v.language = '${preferences.mainLanguage}'`
        }
      GROUP BY ${includeWords ? 'v.global_index, v.verse_index, v.subtitle, v.content, v.footer, v.language' : 'v.global_index, v.verse_index, v.subtitle, v.content, v.footer, v.language'}
      ORDER BY
        v.verse_index ASC,
        CASE
          WHEN v.language = '${preferences.mainLanguage}' THEN 0
          ${
            preferences.includeSecondaryLanguage
              ? `WHEN v.language = '${preferences.secondaryLanguage}' THEN 1`
              : ''
          }
          ELSE 2
        END
    `;

    const verses: VerseWithWords[] = await this.prisma.$queryRawUnsafe(query);

    const ans = {
      ...(preferences.mainLanguageIncludeChapterAudio && {
        chapterAudio: this.audio.buildChapterAudioUrl({
          bitrate: preferences.mainLanguageAudioBitrate,
          edition: preferences.mainLanguageAudioIdentifier,
          number: index,
        }),
      }),
      verses: preferences.mainLanguageIncludeVerseAudio
        ? verses.map((v) => ({
            ...v,
            verseAudio: this.audio.buildVerseByVerseAudioUrl({
              bitrate: preferences.mainLanguageAudioBitrate,
              edition: preferences.mainLanguageAudioIdentifier,
              chapterNumber: index,
              verseNumber: v.verse_index,
            }),
          }))
        : verses,
    };

    return ans;
  }

  public async getQuranFromQuery({
    query,
    preferences,
  }: {
    query: string;
    preferences: QuranFetchPreferences;
  }) {
    const t0 = performance.now();

    this.logger.log({
      message: 'Started parsing Quran query',
      context: 'getQuran',
      query,
    });

    const parsedRequest = this.parseQuranQuery(query);

    const t1 = performance.now();

    this.logger.log({
      message: 'Finished parsing query',
      context: 'getQuran',
      durationMs: (t1 - t0).toFixed(2),
      parsedCount: parsedRequest.length,
    });

    const response = await this.injectVerseData({ parsedRequest, preferences });

    const t2 = performance.now();

    this.logger.log({
      message: 'Fetched verses with preferences',
      context: 'getQuran',
      durationMs: (t2 - t1).toFixed(2),
      preferences,
    });

    this.logger.log({
      message: 'Total time for getQuran',
      context: 'getQuran',
      durationMs: (t2 - t0).toFixed(2),
    });

    return response;
  }
}

type QuranRef =
  | { chapterNum: number; type: 'chapter' }
  | { chapterNum: number; verse: number; type: 'verse' }
  | { chapterNum: number; start: number; end: number; type: 'range' };
