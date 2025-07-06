import { Verses, Words } from '@prisma/client';

export type ChapterResponseType = {
  verses: Omit<Verses, 'uuid' | 'chapter_index' | 'chapter_title'>[];
  words: Omit<Words, 'uuid' | 'language' | 'chapter_index' | 'versesUuid'>[];
  chapterAudio?: string;
};

export type ChapterControllerResponesType =
  | ChapterResponseType
  | {
      success: boolean;
      message: string;
    };
