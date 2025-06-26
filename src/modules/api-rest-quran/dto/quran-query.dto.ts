// import { IsString, Matches } from 'class-validator';
// import { regex } from '../regex.constant';
import { ApiProperty } from '@nestjs/swagger';
import { ApiPropertyOptional } from '@nestjs/swagger';
import { IsOptional, IsString } from 'class-validator';
import { LanguageCodes } from '../api-rest-quran.constant';

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

  @ApiPropertyOptional({
    example: 'en',
    description: 'Primary language of the quran',
    enum: [
      'ar',
      'ar-clean',
      'ar-root',
      'en',
      'tr',
      'fr',
      'ta',
      'hi',
      'tr',
      'fa',
      'ru',
      'sv',
      'de',
      'id',
    ],
  })
  @IsOptional()
  @IsString()
  mainLanguage?: LanguageCodes;

  @ApiPropertyOptional({
    example: 'en',
    description: 'Secondary language of the quran',
    enum: [
      'ar',
      'ar-clean',
      'ar-root',
      'en',
      'tr',
      'fr',
      'ta',
      'hi',
      'tr',
      'fa',
      'ru',
      'sv',
      'de',
      'id',
    ],
  })
  @IsOptional()
  @IsString()
  secondaryLanguage?: LanguageCodes;
}
