type ParsedQueryItem =
  | {
      chapterNum: number;
      verseNum?: number;
      startingVerseNum?: number;
      endingVerseNum?: number;
      errors?: string[];
      queryWithError?: string;
    }
  | {
      errors: string[];
      queryWithError: string;
    };

export type ParsedQueryList = ParsedQueryItem[] | undefined;
