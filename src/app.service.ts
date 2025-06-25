import { Injectable } from '@nestjs/common';
import { LoggerService } from './modules/logger/logger.service';

@Injectable()
export class AppService {
  constructor(private readonly logger: LoggerService) {}
  getHello(): string {
    this.logger.log('Hello Worlded someone.');
    return 'Hello World!';
  }
}
