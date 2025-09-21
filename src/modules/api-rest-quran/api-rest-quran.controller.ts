import {
  Controller,
  Get,
  Param,
  Query,
  UseInterceptors,
  UsePipes,
  BadRequestException,
} from '@nestjs/common';
import {
  ApiOkResponse,
  ApiOperation,
  ApiParam,
  ApiQuery,
  ApiTags,
  getSchemaPath,
  ApiResponse,
} from '@nestjs/swagger';
import {
  ZodSerializerDto,
  ZodSerializerInterceptor,
  ZodValidationPipe,
} from 'nestjs-zod';
import { performance } from 'perf_hooks';
import { ApiRestQuranService } from './api-rest-quran.service';
import { LoggerService } from '../logger/logger.service';
import { ApiRestaudioService } from './api-restaudio/api-restaudio.service';
import {
  ResponseDto,
  ResponseChapterDto,
  ResponseVerseListDto,
} from './dto/response.dto';
import { SCHEMA_RESPONSE } from 'wikisubmission-sdk';
import {
  ClientPreferencesDto,
  ClientPreferencesType,
  PaginationDto,
  RequestDto,
} from './dto/common';

@ApiTags('quran')
@Controller('api-rest-quran')
@UseInterceptors(new ZodSerializerInterceptor(SCHEMA_RESPONSE))
export class ApiRestQuranController {
  constructor(
    private readonly quranService: ApiRestQuranService,
    private readonly logger: LoggerService,
    private readonly audioService: ApiRestaudioService,
  ) {}

  @Get('quran/:query')
  @ApiParam({
    name: 'query',
    type: String,
    required: true,
    description: 'Quran query (e.g., 1:1-7, 2:255, 1,2:1-5)',
    example: '1:1-7',
  })
  @ApiOperation({
    summary: 'Fetch Quran data (chapter or verse list) using query string',
    description: `
    Supported query formats:
    - "1" → entire chapter
    - "1:1" → single verse
    - "1:1-7" → verse range
    - "1,2:1-5,3:10" → multiple chapters/verses/ranges
    - "search text" → text search
    `,
  })
  @ApiOkResponse({
    description: 'Returns a discriminated response (CHAPTER or VERSE_LIST)',
    schema: {
      oneOf: [
        { $ref: getSchemaPath(ResponseChapterDto) },
        { $ref: getSchemaPath(ResponseVerseListDto) },
      ],
    },
  })
  @ApiResponse({
    status: 400,
    description: 'Invalid query format or parameters',
  })
  // @ZodSerializerDto(ResponseChapterDto || ResponseVerseListDto)
  @UsePipes(new ZodValidationPipe())
  async getQuran(
    @Param('query') query: string,
    @Query() preferencesDto: ClientPreferencesDto,
    @Query() pagination?: PaginationDto,
  ): Promise<ResponseDto> {
    const t0 = performance.now();
    const preferences = preferencesDto as unknown as ClientPreferencesType;

    this.logger.log({
      message: `Fetching Quran data for query: ${query}`,
      context: 'ApiRestQuranController.getQuran',
      query,
      preferences: JSON.stringify(preferences),
      pagination: JSON.stringify(pagination),
    });

    const t: string = preferences.main.language.language;

    try {
      const request: RequestDto = {
        query,
        preferences,
        pagination,
      };

      const result = await this.quranService.getQuranFromQuery(request);

      const t1 = performance.now();
      this.logger.log({
        message: `Quran query completed successfully`,
        context: 'ApiRestQuranController.getQuran',
        durationMs: (t1 - t0).toFixed(2),
        resultType: result.type,
      });

      return result;
    } catch (error) {
      const t1 = performance.now();
      const errorMessage =
        error instanceof Error ? error.message : 'Unknown error';

      this.logger.error({
        message: `Error processing Quran query`,
        context: 'ApiRestQuranController.getQuran',
        durationMs: (t1 - t0).toFixed(2),
        error: errorMessage,
        query,
      });

      if (
        errorMessage.includes('out of range') ||
        errorMessage.includes('Invalid')
      ) {
        throw new BadRequestException(errorMessage);
      }

      throw error;
    }
  }

  @Get('chapter/:index')
  @ApiParam({
    name: 'index',
    type: Number,
    required: true,
    description: 'Chapter number (1-114)',
    example: 1,
  })
  @ApiOperation({
    summary: 'Fetch a specific Quran chapter by index',
  })
  @ApiOkResponse({
    description: 'Returns a chapter response',
    type: ResponseChapterDto,
  })
  @ApiResponse({
    status: 400,
    description: 'Invalid chapter index (must be 1-114)',
  })
  @UsePipes(new ZodValidationPipe())
  async getChapter(
    @Param('index') index: number,
    @Query() preferences: ClientPreferencesDto,
    @Query() pagination: PaginationDto,
  ): Promise<ResponseDto> {
    const t0 = performance.now();

    // Validate chapter index
    const chapterIndex = Number(index);
    if (chapterIndex < 1 || chapterIndex > 114) {
      throw new BadRequestException('Chapter index must be between 1 and 114');
    }

    this.logger.log({
      message: `Fetching chapter ${chapterIndex}`,
      context: 'ApiRestQuranController.getChapter',
      chapterIndex,
    });

    try {
      const request: RequestDto = {
        query: chapterIndex.toString(),
        preferences,
        pagination,
      };

      const result = await this.quranService.getQuranFromQuery(request);

      const t1 = performance.now();
      this.logger.log({
        message: `Chapter ${chapterIndex} fetched successfully`,
        context: 'ApiRestQuranController.getChapter',
        durationMs: (t1 - t0).toFixed(2),
      });

      return result;
    } catch (error) {
      const t1 = performance.now();
      const errorMessage =
        error instanceof Error ? error.message : 'Unknown error';

      this.logger.error({
        message: `Error fetching chapter ${chapterIndex}`,
        context: 'ApiRestQuranController.getChapter',
        durationMs: (t1 - t0).toFixed(2),
        error: errorMessage,
      });

      throw error;
    }
  }

  @Get('random-verse')
  @ApiOperation({
    summary: 'Fetch a random verse or daily verse',
  })
  @ApiQuery({
    name: 'date',
    required: false,
    type: String,
    description:
      'ISO date string for daily verse (e.g., 2025-07-11). If omitted, returns truly random verse.',
    example: '2025-07-11',
  })
  @ApiOkResponse({
    description: 'Returns a single verse',
    type: ResponseVerseListDto,
  })
  @UsePipes(new ZodValidationPipe())
  async getRandomVerse(
    @Query() preferences: ClientPreferencesDto,
    @Query() pagination: PaginationDto,
    @Query('date') dateString?: string,
  ): Promise<ResponseDto> {
    const t0 = performance.now();

    let date: Date | undefined;
    if (dateString) {
      date = new Date(dateString);
      if (isNaN(date.getTime())) {
        throw new BadRequestException(
          'Invalid date format. Use ISO format (YYYY-MM-DD)',
        );
      }
    }

    this.logger.log({
      message: `Fetching ${date ? 'daily' : 'random'} verse`,
      context: 'ApiRestQuranController.getRandomVerse',
      date: date?.toISOString(),
    });

    try {
      const result = await this.quranService.getRandomVerse({
        preferences,
        pagination,
        date,
      });

      const t1 = performance.now();
      this.logger.log({
        message: `Random verse fetched successfully`,
        context: 'ApiRestQuranController.getRandomVerse',
        durationMs: (t1 - t0).toFixed(2),
      });

      return result;
    } catch (error) {
      const t1 = performance.now();
      const errorMessage =
        error instanceof Error ? error.message : 'Unknown error';

      this.logger.error({
        message: `Error fetching random verse`,
        context: 'ApiRestQuranController.getRandomVerse',
        durationMs: (t1 - t0).toFixed(2),
        error: errorMessage,
      });

      throw error;
    }
  }

  @Get('random-chapter')
  @ApiOperation({
    summary: 'Fetch a random chapter or daily chapter',
  })
  @ApiQuery({
    name: 'date',
    required: false,
    type: String,
    description:
      'ISO date string for daily chapter (e.g., 2025-07-11). If omitted, returns truly random chapter.',
    example: '2025-07-11',
  })
  @ApiOkResponse({
    description: 'Returns a chapter response',
    type: ResponseChapterDto,
  })
  @UsePipes(new ZodValidationPipe())
  async getRandomChapter(
    @Query() preferences: ClientPreferencesDto,
    @Query() pagination: PaginationDto,
    @Query('date') dateString?: string,
  ): Promise<ResponseDto> {
    const t0 = performance.now();

    let date: Date | undefined;
    if (dateString) {
      date = new Date(dateString);
      if (isNaN(date.getTime())) {
        throw new BadRequestException(
          'Invalid date format. Use ISO format (YYYY-MM-DD)',
        );
      }
    }

    this.logger.log({
      message: `Fetching ${date ? 'daily' : 'random'} chapter`,
      context: 'ApiRestQuranController.getRandomChapter',
      date: date?.toISOString(),
    });

    try {
      const result = await this.quranService.getRandomChapter({
        preferences,
        pagination,
        date,
      });

      const t1 = performance.now();
      this.logger.log({
        message: `Random chapter fetched successfully`,
        context: 'ApiRestQuranController.getRandomChapter',
        durationMs: (t1 - t0).toFixed(2),
      });

      return result;
    } catch (error) {
      const t1 = performance.now();
      const errorMessage =
        error instanceof Error ? error.message : 'Unknown error';

      this.logger.error({
        message: `Error fetching random chapter`,
        context: 'ApiRestQuranController.getRandomChapter',
        durationMs: (t1 - t0).toFixed(2),
        error: errorMessage,
      });

      throw error;
    }
  }

  @Get('audio/metadata/chapters')
  @ApiOperation({
    summary: 'Get available chapter audio metadata',
  })
  @ApiOkResponse({
    description: 'Returns chapter audio availability metadata',
  })
  async getChapterAudioMetadata() {
    this.logger.log({
      message: 'Fetching chapter audio metadata',
      context: 'ApiRestQuranController.getChapterAudioMetadata',
    });

    return this.audioService.fetchChapterAudioAvailability();
  }

  @Get('audio/metadata/verses')
  @ApiOperation({
    summary: 'Get available verse-by-verse audio metadata',
  })
  @ApiOkResponse({
    description: 'Returns verse audio availability metadata',
  })
  async getVerseAudioMetadata() {
    this.logger.log({
      message: 'Fetching verse audio metadata',
      context: 'ApiRestQuranController.getVerseAudioMetadata',
    });

    return this.audioService.fetchVerseAudioAvailability();
  }
}
