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
  ApiQuery,
  ApiResponse,
  ApiTags,
  getSchemaPath,
} from '@nestjs/swagger';
import { LoggerService } from '../logger/logger.service';
import {
  QuranResponseChapter,
  QuranResponseVerse,
  QuranResponseVerses,
} from './dto/quran-response.dto';
import { ApiRestaudioService } from './api-restaudio/api-restaudio.service';
import { AudioResponseDto } from './dto/audio-response.dto';
import { QuranQueryParamsDto } from './dto/quran-query.dto';
import { performance } from 'perf_hooks';
import { ChapterPreferencesDto } from './dto/quran-chapters.dto';
import { ChapterControllerResponesType } from './types';

@ApiTags('quran')
@Controller('api-rest-quran')
export class ApiRestQuranController {
  constructor(
    private readonly Utilities: ApiRestQuranService,
    private readonly Logger: LoggerService,
    private readonly audioService: ApiRestaudioService,
  ) {}

  @ApiExtraModels(QuranResponseVerse, QuranResponseVerses, QuranResponseChapter)
  @ApiQuery({
    name: 'query',
    type: String || Number,
    required: true,
    description: 'Quran query: 1 or 7',
  })
  @ApiOkResponse({
    description: 'Returns a list of chapters, verses, or verse ranges',
    schema: {
      type: 'array',
      items: {
        oneOf: [
          { $ref: getSchemaPath(QuranResponseChapter) },
          { $ref: getSchemaPath(QuranResponseVerse) },
          { $ref: getSchemaPath(QuranResponseVerses) },
        ],
      },
    },
  })
  @Get('quran')
  @UsePipes(new ValidationPipe({ transform: true }))
  getQuran(@Query() queryParams: QuranQueryParamsDto) {
    const start = performance.now();
    this.Logger.log({
      message: `getQuran, with these params ${JSON.stringify(queryParams)}`,
      context: 'ApiRestQuranController.getQuran',
    });
    return;
    // return this.Utilities.getQuranFromQuery({ query: queryParams }).then(
    //   (result) => {
    //     const end = performance.now();
    //     const timeTaken = (end - start).toFixed(2);

    //     this.Logger.log({
    //       message: `getQuran completed in ${timeTaken}ms`,
    //       context: 'ApiRestQuranController.getQuran',
    //     });

    //     return result;
    //   },
    // );
  }

  // @UsePipes(new ValidationPipe({ transform: true }))
  @Get('chapter/:index')
  @UsePipes(new ValidationPipe({ transform: true }))
  @ApiOperation({ summary: 'Fetch a Quran chapter with custom preferences' })
  @ApiResponse({ status: 200, description: 'Successfully fetched chapter' })
  getChapterData(
    @Param('index') index: number,
    @Query()
    preferences: ChapterPreferencesDto,
  ): ChapterControllerResponesType | Promise<ChapterControllerResponesType> {
    this.Logger.log(
      `includeWordByWord: ${typeof preferences.includeWordByWord}`,
    );
    if (index <= 0 || index > 114) {
      return {
        success: false,
        message: 'Chapter number must be between 1-114',
      };
    }

    this.Logger.log(`Fetching this chapter: ${index}`);
    this.Logger.log(`With these parameters: ${JSON.stringify(preferences)}`);
    return this.Utilities.getChapter({
      index: Number(index),
      preferences: preferences,
    });
  }

  @ApiOkResponse({ type: QuranResponseVerse })
  @Get('random-verse')
  getRandomVerse() {
    this.Logger.log({
      message: `getRandomVerse`,
      context: 'ApiRestQuranController.getRandomVerse',
    });
  }

  @ApiOkResponse({ type: QuranResponseChapter })
  @Get('random-chapter')
  getRandomChapter() {
    this.Logger.log({
      message: `getRandomChapter`,
      context: 'ApiRestQuranController.getRandomChapter',
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
