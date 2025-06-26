import {
  Controller,
  Get,
  Query,
  UsePipes,
  ValidationPipe,
} from '@nestjs/common';
import { QuranQueryDto } from './dto/quran-query.dto';
import { ApiRestQuranService } from './api-rest-quran.service';
import { ApiQuery, ApiTags } from '@nestjs/swagger';
import { LoggerService } from '../logger/logger.service';

@ApiTags('quran')
@Controller('api-rest-quran')
export class ApiRestQuranController {
  constructor(
    private readonly Utilities: ApiRestQuranService,
    private readonly Logger: LoggerService,
  ) {}

  @Get('quran')
  @UsePipes(new ValidationPipe({ transform: true }))
  @ApiQuery({
    name: 'query',
    type: String || Number,
    required: true,
    description: 'Quran query: 1:1 or 1:1-7 or 1:2, 1:3-4',
  })
  getQuran(@Query('query') query: string | number) {
    this.Logger.log({
      message: `getQuran, ${query}`,
      context: 'ApiRestQuranController.getQuran',
    });
    return this.Utilities.getQuran(String(query));
  }
}
