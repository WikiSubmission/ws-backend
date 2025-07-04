import { ApiProperty, ApiPropertyOptional } from '@nestjs/swagger';
import { IsOptional, IsString } from 'class-validator';
import { Languages, Qiraats, Scripts } from '@prisma/client';

export interface QuranQueryParams {
  /** Quran verse/chapter query. Example: 1:1-7 */
  query: string;

  /** Main language (e.g., ENGLISH) */
  mainLanguage: Languages;

  /** Optional main Qiraat */
  mainQiraat?: Qiraats;

  /** Main display type */
  mainType: 'wordbyword' | 'versebyverse' | 'chapter';

  /** Optional main script type */
  mainScript?: Scripts;

  /** Whether to include a secondary language */
  includeSecondaryLanguage: boolean;

  /** Optional secondary language */
  secondaryLanguage?: Languages;

  /** Optional secondary Qiraat */
  secondaryQiraat?: Qiraats;

  /** Optional secondary display type */
  secondaryType?: 'wordbyword' | 'versebyverse' | 'chapter';

  /** Optional secondary script type */
  secondaryScript?: Scripts;

  /** Whether to include verse-by-verse audio */
  includeVerseByVerseAudio: boolean;

  /** Whether to include chapter-level audio */
  includeChapterAudio: boolean;

  /** Audio identifier (e.g., ar.abdulazizazzahrani) */
  audioIdentifier: string;

  /** Audio bitrate */
  audioBitrate: 128 | 64 | 40 | 32;

  /** Whether to include word-by-word audio */
  includeWordByWord: boolean;

  /** Whether to send full text */
  sendFullText: boolean;
}

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

  @ApiProperty({ example: false, description: 'Include word by word' })
  includeWordByWord!: boolean;

  @ApiProperty({ example: false, description: 'Send full text' })
  sendFullText!: boolean;

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
