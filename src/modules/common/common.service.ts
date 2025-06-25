import { Inject, Injectable, LoggerService } from '@nestjs/common';
import { Logger } from 'winston';
import { WINSTON_LOGGER } from './logger.constants';
@Injectable()
export class CommonService implements LoggerService {
  constructor(@Inject(WINSTON_LOGGER) private readonly logger: Logger) {}
  log(message: string) {
    this.logger.info(message);
  }

  error(message: string, trace?: string) {
    this.logger.error(message, { trace });
  }

  warn(message: string) {
    this.logger.warn(message);
  }
}
