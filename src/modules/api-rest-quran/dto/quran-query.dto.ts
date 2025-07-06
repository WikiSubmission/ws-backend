import { ApiProperty, ApiPropertyOptional } from '@nestjs/swagger';
import { IsBoolean, IsEnum, IsOptional } from 'class-validator';
import { Languages } from '@prisma/client';
import { Transform } from 'class-transformer';

export type QuranFetchPreferences = {
  mainLanguage: Languages;
  mainLanguageIncludeWordByWord: boolean;
  mainLanguageIncludeRootWord: boolean;
  mainLanguageIncludeVerseAudio: boolean;
  mainLanguageIncludeChapterAudio: boolean;
  mainLanguageAudioIdentifier: string;
  mainLanguageAudioBitrate: 128 | 64 | 40 | 32;

  includeSecondaryLanguage: boolean;
  secondaryLanguage?: Languages;
  secondaryLanguageIncludeWordByWord: boolean;
  secondaryLanguageIncludeRootWord: boolean;
  secondaryLanguageIncludeVerseAudio: boolean;
  secondaryLanguageIncludeChapterAudio: boolean;
  secondaryLanguageAudioIdentifier: string;
  secondaryLanguageAudioBitrate: 128 | 64 | 40 | 32;
};

const toBoolean = ({ value }: { value: any }) =>
  value === 'true' || value === true;

export class QuranFetchPreferencesDto {
  @ApiProperty({
    example: 'ENGLISH',
    description: 'Main language',
    enum: Languages,
  })
  @IsEnum(Languages)
  mainLanguage!: Languages;

  @ApiProperty({ example: false, description: 'Include word by word' })
  @Transform(toBoolean)
  @IsBoolean()
  mainLanguageIncludeWordByWord!: boolean;

  @ApiProperty({ example: false, description: 'Include root words' })
  @Transform(toBoolean)
  @IsBoolean()
  mainLanguageIncludeRootWord!: boolean;

  @ApiProperty({ example: false, description: 'Include verse audio' })
  @Transform(toBoolean)
  @IsBoolean()
  mainLanguageIncludeVerseAudio!: boolean;

  @ApiProperty({ example: false, description: 'Include chapter audio' })
  @Transform(toBoolean)
  @IsBoolean()
  mainLanguageIncludeChapterAudio!: boolean;

  @ApiProperty({
    description: 'Audio identifier',
    example: 'ar.abdulazizazzahrani',
  })
  mainLanguageAudioIdentifier!: string;

  @ApiProperty({
    description: 'Audio bitrate',
    example: 128,
    enum: [128, 64, 40, 32],
  })
  mainLanguageAudioBitrate!: 128 | 64 | 40 | 32;

  @ApiProperty({
    example: false,
    description: 'Should a secondary language be sent. ',
  })
  @Transform(toBoolean)
  @IsBoolean()
  includeSecondaryLanguage!: boolean;

  @ApiPropertyOptional({
    example: 'PERSIAN',
    description: 'Secondary language',
    enum: Languages,
  })
  @IsEnum(Languages)
  secondaryLanguage?: Languages;

  @ApiProperty({ example: false, description: 'Include word by word' })
  @Transform(toBoolean)
  @IsBoolean()
  @IsOptional()
  secondaryLanguageIncludeWordByWord!: boolean;

  @ApiProperty({ example: false, description: 'Include root words' })
  @Transform(toBoolean)
  @IsBoolean()
  secondaryLanguageIncludeRootWord!: boolean;

  @ApiProperty({ example: false, description: 'Include verse audio' })
  @Transform(toBoolean)
  @IsBoolean()
  secondaryLanguageIncludeVerseAudio!: boolean;

  @ApiProperty({ example: false, description: 'Include chapter audio' })
  @Transform(toBoolean)
  @IsBoolean()
  secondaryLanguageIncludeChapterAudio!: boolean;

  @ApiProperty({
    description: 'Audio identifier',
    example: 'ar.abdulazizazzahrani',
  })
  secondaryLanguageAudioIdentifier!: string;

  @ApiProperty({
    description: 'Audio bitrate',
    example: 128,
    enum: [128, 64, 40, 32],
  })
  secondaryLanguageAudioBitrate!: 128 | 64 | 40 | 32;
}
