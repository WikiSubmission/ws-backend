import { Languages } from '@prisma/client';

import { ApiProperty, ApiPropertyOptional } from '@nestjs/swagger';
import { IsBoolean, IsEnum, IsOptional, IsString, IsIn } from 'class-validator';
import { Transform } from 'class-transformer';
export class ChapterPreferencesDto {
  @ApiProperty({
    description: 'Include word-by-word breakdown',
    example: false,
    default: false,
  })
  @Transform(({ value }) => {
    return value === 'true' || value === true;
  })
  @IsBoolean()
  @IsOptional()
  includeWordByWord: boolean = false;

  @ApiProperty({
    description: 'Include root word',
    example: false,
    default: false,
  })
  @IsBoolean()
  @Transform(({ value }) => value === 'true' || value === true)
  @IsOptional()
  includeRootWord: boolean = false;

  @ApiProperty({
    description: 'Language of the translation',
    enum: Languages,
    example: 'ENGLISH',
    default: 'ENGLISH',
  })
  @IsEnum(Languages)
  @IsOptional()
  language: Languages = Languages.ENGLISH;

  @ApiProperty({
    description: 'Include chapter-level audio URL',
    example: false,
    default: false,
  })
  @IsBoolean()
  @Transform(({ value }) => value === 'true' || value === true)
  @IsOptional()
  includeChapterAudio: boolean = false;

  @ApiProperty({
    description: 'Include verse-by-verse audio URLs',
    example: true,
    default: false,
  })
  @IsBoolean()
  @Transform(({ value }) => value === 'true' || value === true)
  @IsOptional()
  includeVerseAudio: boolean = false;

  @ApiPropertyOptional({
    description: 'Audio bitrate in kbps',
    enum: [128, 64, 40, 32],
    example: 128,
    default: 64,
  })
  @IsIn([128, 64, 40, 32])
  @Transform(({ value }) => parseInt(value, 10))
  @IsOptional()
  bitrate?: 128 | 64 | 40 | 32 = 64;

  @ApiPropertyOptional({
    description: 'Audio edition identifier',
    example: 'ar.abdulbasitmurattal',
    default: 'ar.abdulbasitmurattal',
  })
  @IsString()
  @IsOptional()
  edition?: string = 'ar.abdulbasitmurattal';
}

export type ChapterPreferences = {
  language: Languages;
  includeWordByWord: boolean;
  includeRootWord: boolean;
  includeChapterAudio: boolean;
  includeVerseAudio: boolean;
  bitrate?: 128 | 64 | 40 | 32;
  edition?: string;
};
