import { createZodDto } from 'nestjs-zod';
import {
  SCHEMA_VERSE_LIST_RESPONSE,
  SCHEMA_CHAPTER_RESPONSE,
  SCHEMA_RESPONSE,
} from 'wikisubmission-sdk';
import z from 'zod';

export type ResponseDto = z.infer<typeof SCHEMA_RESPONSE>;
export class ResponseVerseListDto extends createZodDto(
  SCHEMA_VERSE_LIST_RESPONSE,
) {}
export type ResponseVerseListType = z.infer<typeof SCHEMA_VERSE_LIST_RESPONSE>;
export class ResponseChapterDto extends createZodDto(SCHEMA_CHAPTER_RESPONSE) {}
export type ResponseChapterType = z.infer<typeof SCHEMA_CHAPTER_RESPONSE>;
