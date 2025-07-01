// import { IsString, Matches } from 'class-validator';
// import { regex } from '../regex.constant';
import { ApiProperty } from '@nestjs/swagger';
import { ApiPropertyOptional } from '@nestjs/swagger';
import { IsOptional, IsString } from 'class-validator';
import { Languages, Qiraats, Scripts } from '@prisma/client';

export class QuranQueryDto {
  // // eslint-disable-next-line @typescript-eslint/no-unsafe-call
  // @Matches(regex, {
  //   message: 'Query must be a valid format like 1, 1:2, 2:4-6, 5:1,6',
  // })
  @ApiProperty({
    description: 'Query string like 1:1, 1:2-4',
    example: '1:1-7',
  })
  query!: string;
}

export class QuranQueryParamsDto {
  @ApiPropertyOptional({
    description: 'Quran verse/chapter query. Example: 1:1, 1:2-7',
    example: '1:1-7',
  })
  query!: string;

  @ApiPropertyOptional({
    description: 'Primary language of the quran',
    enum: Languages,
  })
  mainLanguage?: LanguageQueryParamsDto;

  @ApiPropertyOptional({
    description: 'Secondary language of the quran',
  })
  @IsOptional()
  secondaryLanguage?: LanguageQueryParamsDto;

  audioQuery?: AudioQueryParamsDto;
}

class LanguageQueryParamsDto {
  @ApiPropertyOptional({
    example: 'ENGLISH',
    description: 'Language',
    enum: Languages,
  })
  @IsOptional()
  @IsString()
  language?: Languages;

  @ApiPropertyOptional({
    example: 'HAFS',
    description: 'Qiraat',
    enum: Qiraats,
  })
  @IsOptional()
  @IsString()
  qiraat?: Qiraats;

  scripts?: Scripts;

  @ApiPropertyOptional({
    example: 'versebyverse',
    description: 'Display type',
    enum: ['wordbyword', 'versebyverse', 'chapter'],
  })
  @IsOptional()
  @IsString()
  type?: 'wordbyword' | 'versebyverse' | 'chapter';
}

class AudioQueryParamsDto {
  @ApiPropertyOptional({ example: false, description: 'Include audio links' })
  @IsOptional()
  audio?: boolean;

  @ApiPropertyOptional({
    description: 'Audio identifier',
    example: 'ar.abdulazizazzahrani',
  })
  audioIdentifier?: string;

  @ApiPropertyOptional({
    description: 'Audio bitrate',
    example: 128,
  })
  audioBitrate?: number;
}
