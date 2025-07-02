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
  @ApiPropertyOptional({
    example: 'ENGLISH',
    description: 'Main language',
    enum: Languages,
  })
  mainLanguage?: Languages;

  @ApiPropertyOptional({
    example: 'HAFS',
    description: 'Main Qiraat',
    enum: Qiraats,
  })
  @IsOptional()
  @IsString()
  mainQiraat?: Qiraats;

  @ApiPropertyOptional({
    example: 'versebyverse',
    description: 'Main display type',
    enum: ['wordbyword', 'versebyverse', 'chapter'],
  })
  @IsOptional()
  @IsString()
  mainType?: 'wordbyword' | 'versebyverse' | 'chapter';

  @ApiPropertyOptional({
    example: 'UTHMANIC',
    description: 'Main script type',
    enum: Scripts,
  })
  @IsOptional()
  @IsString()
  mainScript?: Scripts;

  // Secondary Language
  @ApiPropertyOptional({
    example: false,
    description: 'Should a secondary language be sent. ',
  })
  includeSecondaryLanguage?: boolean;

  @ApiPropertyOptional({
    example: 'FRENCH',
    description: 'Secondary language',
    enum: Languages,
  })
  secondaryLanguage?: Languages;

  @ApiPropertyOptional({
    example: 'WARSH',
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
  @ApiPropertyOptional({ example: false, description: 'Include audio' })
  @IsOptional()
  includeAudio?: boolean;

  @ApiPropertyOptional({
    description: 'Audio identifier',
    example: 'ar.abdulazizazzahrani',
  })
  audioIdentifier?: string;

  @ApiPropertyOptional({
    description: 'Audio bitrate',
    example: 128,
    enum: [128, 64, 40, 32],
  })
  audioBitrate?: 128 | 64 | 40 | 32;
}
