import { Verses, Words } from '@prisma/client';

// export type VerseWithWords = Omit<
//   Verses,
//   'uuid' | 'chapter_index' | 'chapter_title'
// > & {
//   Words?: Words[];
// };

export type VerseWithWords = {
  global_index: number;
  verse_index: number;
  subtitle: string | null;
  content: string;
  footer: string | null;
  language: string;
  Words: {
    verse_index: number;
    word_index: number;
    content: string | null;
    root: string | null;
  }[];
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
