import { Verses, Words } from '@prisma/client';

export type VerseWithWords = Omit<
  Verses,
  'uuid' | 'chapter_index' | 'chapter_title'
> & {
  Words?: Words[];
};

export type ChapterResponseType = {
  verses: VerseWithWords[];
  chapterAudio?: string;
};

export type ChapterControllerResponesType =
  | ChapterResponseType
  | {
      success: boolean;
      message: string;
    };
