import { ApiProperty, ApiPropertyOptional } from '@nestjs/swagger';
import { IsOptional, IsString } from 'class-validator';
import { Languages, Qiraats, Scripts } from '@prisma/client';

export class QuranQueryParamsDto {
  @ApiPropertyOptional({
    description: 'Quran verse/chapter query. Example: 1:1, 1:2-7',
    example: '1:1-7',
  })
  query!: string;

  // Main Language
  @ApiProperty({
    example: 'ENGLISH',
    description: 'Main language',
    enum: Languages,
  })
  mainLanguage!: Languages;

  @ApiPropertyOptional({
    description: 'Main Qiraat',
    enum: Qiraats,
  })
  @IsOptional()
  @IsString()
  mainQiraat?: Qiraats;

  @ApiProperty({
    example: 'versebyverse',
    description: 'Main display type',
    enum: ['wordbyword', 'versebyverse', 'chapter'],
  })
  @IsOptional()
  @IsString()
  mainType!: 'wordbyword' | 'versebyverse' | 'chapter';

  @ApiPropertyOptional({
    example: 'UTHMANIC',
    description: 'Main script type',
    enum: Scripts,
  })
  @IsOptional()
  @IsString()
  mainScript?: Scripts;

  // Secondary Language
  @ApiProperty({
    example: false,
    description: 'Should a secondary language be sent. ',
  })
  includeSecondaryLanguage!: boolean;

  @ApiPropertyOptional({
    example: 'PERSIAN',
    description: 'Secondary language',
    enum: Languages,
  })
  secondaryLanguage?: Languages;

  @ApiPropertyOptional({
    description: 'Secondary Qiraat',
    enum: Qiraats,
  })
  @IsOptional()
  @IsString()
  secondaryQiraat?: Qiraats;

  @ApiPropertyOptional({
    example: 'chapter',
    description: 'Secondary display type',
    enum: ['wordbyword', 'versebyverse', 'chapter'],
  })
  @IsOptional()
  @IsString()
  secondaryType?: 'wordbyword' | 'versebyverse' | 'chapter';

  @ApiPropertyOptional({
    example: 'INDOPAK',
    description: 'Secondary script type',
    enum: Scripts,
  })
  @IsOptional()
  @IsString()
  secondaryScript?: Scripts;

  // Audio
  @ApiProperty({ example: false, description: 'Include verse audio' })
  includeVerseByVerseAudio!: boolean;

  @ApiProperty({ example: false, description: 'Include chapter audio' })
  includeChapterAudio!: boolean;

  @ApiProperty({
    description: 'Audio identifier',
    example: 'ar.abdulazizazzahrani',
  })
  audioIdentifier!: string;

  @ApiProperty({
    description: 'Audio bitrate',
    example: 128,
    enum: [128, 64, 40, 32],
  })
  audioBitrate!: 128 | 64 | 40 | 32;
}
