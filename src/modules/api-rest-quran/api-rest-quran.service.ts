import { Injectable } from '@nestjs/common';
import { randomInt } from 'crypto';
import seedrandom from 'seedrandom';
import { z } from 'zod';

import { PrismaService } from '../prisma/prisma.service';
import { LoggerService } from '../logger/logger.service';
import { ApiRestaudioService } from './api-restaudio/api-restaudio.service';

// Import Zod schemas and types
import {
  SCHEMA_REQUEST,
  SCHEMA_RESPONSE,
  SCHEMA_CLIENT_PREFERENCES,
  SCHEMA_PAGINATION_CLIENT,
  SCHEMA_PAGINATION_SERVER,
  SCHEMA_CHAPTER_RESPONSE,
  SCHEMA_VERSE_LIST_RESPONSE,
  SCHEMA_LANGUAGES,
} from 'wikisubmission-sdk';

// Define types from schemas
type RequestDto = z.infer<typeof SCHEMA_REQUEST>;
type ResponseDto = z.infer<typeof SCHEMA_RESPONSE>;
type ClientPreferencesType = z.infer<typeof SCHEMA_CLIENT_PREFERENCES>;
type PaginationClientType = z.infer<typeof SCHEMA_PAGINATION_CLIENT>;
type PaginationServerType = z.infer<typeof SCHEMA_PAGINATION_SERVER>;
type LanguageType = z.infer<typeof SCHEMA_LANGUAGES>;

// Internal types for query parsing
type ParsedQuery =
  | {
      type: 'valid';
      chapterNum: number;
      verseNum?: number;
      startingVerseNum?: number;
      endingVerseNum?: number;
    }
  | { type: 'text'; text: string }
  | { type: 'invalid'; queryWithError: string; errors: string[] };

type ParsedQueryList = ParsedQuery[];

// Database result types
interface VerseDbResult {
  idx?: number;
  globalIndex: number;
  verseIndex: number;
  chapterIndex: number;
  title?: string;
  subtitle?: string;
  content: string;
  footer?: string;
  language: string;
  words?: Array<{
    verseIndex: number;
    wordIndex: number;
    content?: string;
    root?: string;
  }>;
}

@Injectable()
export class ApiRestQuranService {
  constructor(
    private readonly prisma: PrismaService,
    private readonly logger: LoggerService,
    private readonly audio: ApiRestaudioService,
  ) {}

  // Chapter verse counts lookup
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
   * Main entry point for Quran queries
   */
  public async getQuranFromQuery(request: RequestDto): Promise<ResponseDto> {
    const t0 = performance.now();

    this.logger.log({
      message: 'Started parsing Quran query',
      context: 'ApiRestQuranService.getQuranFromQuery',
      query: request.query,
    });

    // Parse the query string
    const parsedRequest = this.parseQuranQuery(request.query);

    const t1 = performance.now();
    this.logger.log({
      message: 'Finished parsing query',
      context: 'ApiRestQuranService.getQuranFromQuery',
      durationMs: (t1 - t0).toFixed(2),
      parsedCount: parsedRequest.length,
    });

    // Check for invalid queries
    const invalidQueries = parsedRequest.filter((q) => q.type === 'invalid');
    if (invalidQueries.length > 0) {
      const errors = invalidQueries.flatMap((q) =>
        q.type === 'invalid' ? q.errors : [],
      );
      throw new Error(`Invalid query: ${errors.join(', ')}`);
    }

    // Inject verse data with preferences and pagination
    const response = await this.injectVerseData({
      parsedRequest,
      preferences: request.preferences,
      pagination: request.pagination,
    });

    const t2 = performance.now();
    this.logger.log({
      message: 'Completed Quran query',
      context: 'ApiRestQuranService.getQuranFromQuery',
      durationMs: (t2 - t0).toFixed(2),
      responseType: response.type,
    });

    return response;
  }

  /**
   * Get a random verse (or daily verse if date provided)
   */
  public async getRandomVerse({
    preferences,
    pagination,
    date,
  }: {
    preferences: ClientPreferencesType;
    pagination: PaginationClientType;
    date?: Date;
  }): Promise<ResponseDto> {
    let chapterIndex: number;
    let verseIndex: number;

    if (date) {
      const seed = date.toISOString().slice(0, 10);
      const rng = seedrandom(seed);
      chapterIndex = Math.floor(rng() * 114) + 1;
      verseIndex = Math.floor(rng() * this.verseCounts[chapterIndex]) + 1;
    } else {
      chapterIndex = randomInt(1, 115);
      verseIndex = randomInt(1, this.verseCounts[chapterIndex] + 1);
    }

    const request: RequestDto = {
      query: `${chapterIndex}:${verseIndex}`,
      preferences,
      pagination,
    };

    return this.getQuranFromQuery(request);
  }

  /**
   * Get a random chapter (or daily chapter if date provided)
   */
  public async getRandomChapter({
    preferences,
    pagination,
    date,
  }: {
    preferences: ClientPreferencesType;
    pagination: PaginationClientType;
    date?: Date;
  }): Promise<ResponseDto> {
    let chapterIndex: number;

    if (date) {
      const seed = date.toISOString().slice(0, 10);
      const rng = seedrandom(seed);
      chapterIndex = Math.floor(rng() * 114) + 1;
    } else {
      chapterIndex = randomInt(1, 115);
    }

    const request: RequestDto = {
      query: chapterIndex.toString(),
      preferences,
      pagination,
    };

    return this.getQuranFromQuery(request);
  }

  /**
   * Parse query string into structured format
   */
  private parseQuranQuery(input: string): ParsedQueryList {
    const results: ParsedQueryList = [];
    const parts = input.split(',').map((p) => p.trim());
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
        // Treat as text search
        results.push({
          type: 'text',
          text: part,
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
        // Whole chapter
        results.push({ type: 'valid', chapterNum });
      } else if (!endVerseStr) {
        // Single verse
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
        // Verse range
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

  /**
   * Fetch verse data from database based on parsed query
   */
  private async injectVerseData({
    parsedRequest,
    preferences,
    pagination,
  }: {
    parsedRequest: ParsedQueryList;
    preferences: ClientPreferencesType;
    pagination?: PaginationClientType;
  }): Promise<ResponseDto> {
    // Filter out text queries for now (TODO: implement text search)
    const validQueries = parsedRequest.filter(
      (q) => q.type === 'valid',
    ) as Array<ParsedQuery & { type: 'valid' }>;

    if (validQueries.length === 0) {
      throw new Error('No valid queries found');
    }

    const includeWords =
      preferences.main.language.includeWordByWord ||
      preferences.main.language.includeRootByRoot;

    // Build input tuples for SQL
    const inputTuples: string[] = [];
    validQueries.forEach((item, idx) => {
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
    });

    if (inputTuples.length === 0) {
      throw new Error('No valid input tuples generated');
    }

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

    const mainLang = preferences.main.language.language;
    const secondaryLang = preferences?.secondary?.language?.language;

    const languageFilter = secondaryLang
      ? `v.language IN ('${mainLang}', '${secondaryLang}')`
      : `v.language = '${mainLang}'`;

    const joinWordsSQL = includeWords
      ? `
        , COALESCE(
            json_agg(
              json_build_object(
                'verseIndex', w.verse_index,
                'wordIndex', w.word_index
                ${preferences.main.language.includeWordByWord ? `, 'content', w.content` : ''}
                ${preferences.main.language.includeRootByRoot ? `, 'root', w.root` : ''}
              )
              ORDER BY w.word_index
            ) FILTER (WHERE w.word_index IS NOT NULL),
            '[]'
          ) AS words
      `
      : '';

    const wordJoinSQL = includeWords
      ? `LEFT JOIN "Words" w ON w."versesUuid" = v.uuid AND w.language = '${mainLang}'`
      : '';

    const sql = `
      ${inputCTE}
      SELECT
        io.idx,
        v.global_index AS "globalIndex",
        v.verse_index AS "verseIndex",
        v.chapter_index AS "chapterIndex",
        v.chapter_title AS title,
        v.subtitle,
        v.content,
        v.footer,
        v.language
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
      GROUP BY io.idx, v.global_index, v.verse_index, v.chapter_index, v.chapter_title, v.subtitle, v.content, v.footer, v.language
      ORDER BY io.idx, v.verse_index,
        CASE
          WHEN v.language = '${mainLang}' THEN 0
          ${secondaryLang ? `WHEN v.language = '${secondaryLang}' THEN 1` : ''}
          ELSE 2
        END
    `;

    const verses: VerseDbResult[] = await this.prisma.$queryRawUnsafe(sql);

    // Transform database results to Zod schema format
    const transformedVerses = verses.map((v) => ({
      globalIndex: v.globalIndex,
      chapterIndex: v.chapterIndex,
      verseIndex: v.verseIndex,
      subtitle: v.subtitle || undefined,
      content: v.content,
      footer: v.footer || undefined,
      // Add audio if requested
      ...(preferences.main.audioSettings.includeVerseAudio && {
        audio: {
          link: this.audio.buildVerseByVerseAudioUrl({
            bitrate: parseInt(preferences.main.audioSettings.bitrate),
            edition: preferences.main.audioSettings.identifier,
            chapterNumber: v.chapterIndex,
            verseNumber: v.verseIndex,
          }),
          lengthSeconds: 0, // TODO: Get actual length
          reciterName: '', // TODO: Get reciter name
          identifier: preferences.main.audioSettings.identifier,
        },
      }),
    }));

    // Create pagination response
    // const paginationResponse: PaginationServerType | null = {
    //   startCursor: pagination.startCursor,
    //   endCursor: pagination.endCursor,
    //   pageSize: pagination.pageSize,
    //   hasNextPage: false, // TODO: Implement actual pagination logic
    //   hasPreviousPage: false, // TODO: Implement actual pagination logic
    // };

    // Determine if this is a single chapter or verse list
    const isSingleChapter =
      validQueries.length === 1 &&
      validQueries[0].verseNum === undefined &&
      validQueries[0].startingVerseNum === undefined;

    if (isSingleChapter && verses.length > 0) {
      const chapterIndex = validQueries[0].chapterNum;
      const chapterTitle = verses[0].title || `Chapter ${chapterIndex}`;

      return SCHEMA_CHAPTER_RESPONSE.parse({
        type: 'CHAPTER',
        data: {
          chapterIndex,
          title: chapterTitle,
          verses: transformedVerses,
          // Add chapter audio if requested
          ...(preferences.main.audioSettings.includeChapterAudio && {
            audio: {
              link: this.audio.buildChapterAudioUrl({
                bitrate: parseInt(preferences.main.audioSettings.bitrate),
                edition: preferences.main.audioSettings.identifier,
                number: chapterIndex,
              }),
              lengthSeconds: 0, // TODO: Get actual length
              reciterName: '', // TODO: Get reciter name
              identifier: preferences.main.audioSettings.identifier,
            },
          }),
        },
        // pagination: paginationResponse,
      });
    } else {
      // Return as verse list
      return SCHEMA_VERSE_LIST_RESPONSE.parse({
        type: 'VERSE_LIST',
        data: transformedVerses,
        // pagination: paginationResponse,
      });
    }
  }

  /**
   * Convert database language codes to Zod enum values
   */
  private mapDatabaseLanguageToZod(dbLanguage: string): LanguageType {
    const languageMap: Record<string, LanguageType> = {
      en: 'ENGLISH',
      ar: 'ARABIC',
      fr: 'FRENCH',
      es: 'SPANISH',
      ta: 'TAMIL',
      ms: 'BAHASA',
      fa: 'PERSIAN',
      tr: 'TURKISH',
      de: 'GERMAN',
      sv: 'SWEDISH',
      ru: 'RUSSIAN',
      transliterated: 'TRANSLITERATED',
    };

    return languageMap[dbLanguage] || 'ENGLISH';
  }

  /**
   * Convert Zod language enum to database language code
   */
  private mapZodLanguageToDatabase(zodLanguage: LanguageType): string {
    const languageMap: Record<LanguageType, string> = {
      ENGLISH: 'en',
      ARABIC: 'ar',
      FRENCH: 'fr',
      SPANISH: 'es',
      TAMIL: 'ta',
      BAHASA: 'ms',
      PERSIAN: 'fa',
      TURKISH: 'tr',
      GERMAN: 'de',
      SWEDISH: 'sv',
      RUSSIAN: 'ru',
      TRANSLITERATED: 'transliterated',
    };

    return languageMap[zodLanguage] || 'en';
  }

  /**
   * Handle text search queries (TODO: Implement full-text search)
   */
  private async handleTextSearch({
    searchText,
    preferences,
    pagination,
  }: {
    searchText: string;
    preferences: ClientPreferencesType;
    pagination: PaginationClientType;
  }): Promise<ResponseDto> {
    // TODO: Implement text search based on preferences.main.language.searchStrategy
    // For now, throw an error
    throw new Error('Text search not yet implemented');
  }

  /**
   * Validate chapter and verse ranges
   */
  private validateChapterVerse(chapterNum: number, verseNum?: number): boolean {
    if (chapterNum < 1 || chapterNum > 114) {
      return false;
    }

    if (verseNum !== undefined) {
      const maxVerse = this.verseCounts[chapterNum];
      return verseNum >= 1 && verseNum <= maxVerse;
    }

    return true;
  }

  /**
   * Get chapter metadata
   */
  public async getChapterMetadata(chapterIndex: number) {
    if (chapterIndex < 1 || chapterIndex > 114) {
      throw new Error('Invalid chapter index');
    }

    // TODO: Implement chapter metadata retrieval
    return {
      chapterIndex,
      verseCount: this.verseCounts[chapterIndex],
      // Add more metadata as needed
    };
  }

  /**
   * Search verses by content (TODO: Implement)
   */
  public async searchVerses({
    query,
    preferences,
    pagination,
  }: {
    query: string;
    preferences: ClientPreferencesType;
    pagination: PaginationClientType;
  }): Promise<ResponseDto> {
    // TODO: Implement verse search with different strategies:
    // - FULL_TEXT_SEARCH
    // - FUZZY
    // - HYBRID

    throw new Error('Verse search not yet implemented');
  }

  /**
   * Get bookmarked verses for a user
   */
  public async getBookmarkedVerses({
    userId,
    preferences,
    pagination,
  }: {
    userId: string;
    preferences: ClientPreferencesType;
    pagination: PaginationClientType;
  }): Promise<ResponseDto> {
    // TODO: Implement bookmark retrieval
    throw new Error('Bookmark retrieval not yet implemented');
  }

  /**
   * Add verse bookmark
   */
  public async addBookmark({
    userId,
    chapterIndex,
    verseIndex,
    content,
  }: {
    userId: string;
    chapterIndex: number;
    verseIndex: number;
    content: string;
  }) {
    if (!this.validateChapterVerse(chapterIndex, verseIndex)) {
      throw new Error('Invalid chapter or verse index');
    }

    // TODO: Implement bookmark creation
    throw new Error('Bookmark creation not yet implemented');
  }

  /**
   * Remove verse bookmark
   */
  public removeBookmark({
    userId,
    chapterIndex,
    verseIndex,
  }: {
    userId: string;
    chapterIndex: number;
    verseIndex: number;
  }) {
    // TODO: Implement bookmark removal
    throw new Error('Bookmark removal not yet implemented');
  }
}
