import {
  Controller,
  Get,
  Query,
  UsePipes,
  ValidationPipe,
} from '@nestjs/common';
import { ApiRestQuranService } from './api-rest-quran.service';
import {
  ApiExtraModels,
  ApiOkResponse,
  ApiQuery,
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
    description: 'Quran query: 1:1 or 1:1-7 or 1:2, 1:3-4',
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
    this.Logger.log({
      message: `getQuran, with these params ${JSON.stringify(queryParams)}`,
      context: 'ApiRestQuranController.getQuran',
    });
    return this.Utilities.getQuranFromQuery({
      query: queryParams.query,
      mainLanguage: queryParams.mainLanguage,
      mainQiraat: queryParams.mainQiraat,
      includeChapterAudio: queryParams.includeChapterAudio,
      includeVerseByVerseAudio: queryParams.includeVerseByVerseAudio,
    });
  }
  @ApiOkResponse({ type: QuranResponseVerse })
  @Get('random-verse')
  getRandomVerse() {
    this.Logger.log({
      message: `getRandomVerse`,
      context: 'ApiRestQuranController.getRandomVerse',
    });
    return this.Utilities.getRandomVerse();
  }

  @ApiOkResponse({ type: QuranResponseChapter })
  @Get('random-chapter')
  getRandomChapter() {
    this.Logger.log({
      message: `getRandomChapter`,
      context: 'ApiRestQuranController.getRandomChapter',
    });
    return this.Utilities.getRandomChapter();
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
