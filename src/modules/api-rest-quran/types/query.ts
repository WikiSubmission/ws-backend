import { Chapter, Quran, Verse, Word } from '@prisma/client';

export type ParsedQueryItem =
  | {
      type: 'valid';
      chapterNum: number;
      verseNum?: number;
      startingVerseNum?: number;
      endingVerseNum?: number;
    }
  | {
      type: 'invalid';
      errors: string[];
      queryWithError: string;
    };

export type ParsedQueryList = ParsedQueryItem[];
export type ValidParsedQueryItem = Extract<ParsedQueryItem, { type: 'valid' }>;
export type InvalidParsedQueryItem = Extract<
  ParsedQueryItem,
  { type: 'invalid' }
>;

export type EnrichedQueryItem = ValidParsedQueryItem & {
  data: {
    type: 'verses';
    fullText?: string;
    audio?: string;
    verseByVerse: Array<{
      verse: {
        number: number;
        content: string;
        secondaryContent?: string | null;
        subtitle?: string | null;
        footnote?: string | null;
        audio?: string;
      };
      words?: Array<{
        number: number;
        content: string;
        root?: string | null;
      }>;
    }>;
  };
};

export type ProcessedQueryItem = EnrichedQueryItem | InvalidParsedQueryItem;

export type VerseWithRelations = Verse & {
  chapter: Chapter & {
    quran: Quran;
  };
  words?: Word[];
};

type ItemWithData = {
  chapterNum: number;
  verseNum?: number;
  startingVerseNum?: number;
  endingVerseNum?: number;
  data?:
    | {
        type: 'chapter';
        chapter: string;
      }
    | {
        type: 'versebyverse';
        verse: Omit<Verse, 'id' | 'chapterId'>;
      }
    | {
        type: 'versebychapter';
        words: Word[];
      };
};

export type ItemsWithData = ItemWithData[];
