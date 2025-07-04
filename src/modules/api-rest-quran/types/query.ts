import { Verse, Word } from '@prisma/client';

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
