import {
  Body,
  Controller,
  Get,
  Param,
  Post,
  Query,
  UsePipes,
  ValidationPipe,
} from '@nestjs/common';
import { ApiRestQuranService } from './api-rest-quran.service';
import {
  ApiExtraModels,
  ApiOkResponse,
  ApiOperation,
  ApiParam,
  ApiQuery,
  ApiResponse,
  ApiTags,
  getSchemaPath,
} from '@nestjs/swagger';
import { LoggerService } from '../logger/logger.service';
import {
  QuranResponseChapter,
  QuranResponseVerse,
} from './dto/quran-response.dto';
import { ApiRestaudioService } from './api-restaudio/api-restaudio.service';
import { AudioResponseDto } from './dto/audio-response.dto';
import { QuranFetchPreferencesDto } from './dto/quran-query.dto';
import { performance } from 'perf_hooks';
import { ChapterControllerResponesType } from './types';
import { Pipeline } from 'ioredis';

@ApiTags('quran')
@Controller('api-rest-quran')
export class ApiRestQuranController {
  constructor(
    private readonly Utilities: ApiRestQuranService,
    private readonly Logger: LoggerService,
    private readonly audioService: ApiRestaudioService,
  ) {}

  @ApiParam({
    name: 'query',
    type: String,
    required: true,
    description: 'Quran verse/chapter query. Example: 1:1,1:2-7',
    example: '1:1-7',
  })
  @ApiOperation({
    summary:
      'Queries the quran using shorthand notation (1:1,1:2-7), with preferences.',
  })
  @ApiOkResponse({
    description: 'Returns a list of chapters, verses, or verse ranges',
  })
  @Get('quran/:query')
  @UsePipes(new ValidationPipe({ transform: true }))
  async getQuran(
    @Param('query') query: string,
    @Query() preferences: QuranFetchPreferencesDto,
  ) {
    const t0 = performance.now();
    this.Logger.log({
      message: `getQuranQuery [${query}], with these params ${JSON.stringify(preferences)}`,
      context: 'ApiRestQuranController.getQuran',
    });
    const t1 = performance.now();
    const result = await this.Utilities.getQuranFromQuery({
      query,
      preferences,
    });
    const t2 = performance.now();

    return result;
  }

  @Get('chapter/:index')
  @UsePipes(new ValidationPipe({ transform: true }))
  @ApiOperation({ summary: 'Fetch a Quran chapter with custom preferences' })
  @ApiResponse({ status: 200, description: 'Successfully fetched chapter' })
  getChapterData(
    @Param('index') index: number,
    @Query()
    preferences: QuranFetchPreferencesDto,
  ): ChapterControllerResponesType | Promise<ChapterControllerResponesType> {
    const t0 = performance.now();
    if (index <= 0 || index > 114) {
      return {
        success: false,
        message: 'Chapter number must be between 1-114',
      };
    }

    const result = this.Utilities.getChapter({
      index: Number(index),
      preferences: preferences,
    });

    const t1 = performance.now();
    this.Logger.log(`getChapterData completed in ${(t1 - t0).toFixed(2)}ms`);
    return result;
  }

  @Get('random-verse')
  @ApiOkResponse({ type: QuranResponseVerse })
  @ApiOperation({
    summary:
      'Fetch a random Quran verse with custom preferences as well as an option to get the daily verse',
  })
  @ApiQuery({
    name: 'date',
    required: false,
    type: String,
    description: 'ISO date string (e.g., 2025-07-11)',
  })
  @ApiQuery({
    name: 'preferences',
    required: false,
    type: QuranFetchPreferencesDto,
    description: 'Custom preferences for fetching a Quran verse',
  })
  getRandomVerse(
    @Query() preferences: QuranFetchPreferencesDto,
    @Query('date') date?: string,
  ) {
    this.Logger.log({
      message: `getRandomVerse`,
      context: 'ApiRestQuranController.getRandomVerse',
    });
    this.Utilities.getRandomVerse({
      ...(date && { date: new Date(date) }),
      preferences: preferences,
    });
  }

  @ApiOkResponse({ type: QuranResponseChapter })
  @Get('random-chapter')
  @ApiOperation({
    summary:
      'Fetch a random Quran chapter with custom preferences as well as an option to get the daily chapter',
  })
  @ApiQuery({
    name: 'preferences',
    required: false,
    type: QuranFetchPreferencesDto,
    description: 'Custom preferences for fetching a Quran verse',
  })
  getRandomChapter(
    @Query() preferences: QuranFetchPreferencesDto,
    @Query('date') date?: string,
  ) {
    this.Logger.log({
      message: `getRandomChapter`,
      context: 'ApiRestQuranController.getRandomChapter',
    });
    this.Utilities.getRandomChapter({
      ...(date && { date: new Date(date) }),
      preferences: preferences,
    });
  }

  @Get('audio-chapter-metadata')
  @ApiOkResponse({ type: AudioResponseDto })
  async fetchChapterAudioAvailability() {
    this.Logger.log({
      message: `fetchChapterAudioAvailability`,
      context: 'ApiRestQuranController.fetchChapterAudioAvailability',
    });
    return this.audioService.fetchChapterAudioAvailability();
  }

  @Get('audio-verse-by-verse-metadata')
  async fetchVerseAudioAvailability(): Promise<Record<string, any>> {
    this.Logger.log({
      message: `fetchVerseAudio`,
      context: 'ApiRestQuranController.fetchVerseAudio',
    });
    return this.audioService.fetchVerseAudioAvailability();
  }

  @Get('chapter-audio')
  fetchChapterAudio(): string {
    this.Logger.log({
      message: `fetchChapterAudio`,
      context: 'ApiRestQuranController.fetchChapterAudio',
    });
    return this.audioService.fetchChapterAudio({
      chapter: 1,
      edition: 'ar.alafasy',
      bitrate: 128,
    });
  }
}
