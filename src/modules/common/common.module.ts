import { Global, Module } from '@nestjs/common';
import { WinstonModule } from 'nest-winston';
import { LoggerFactory } from './logger.factory';
import { WINSTON_LOGGER } from './logger.constants';

@Global()
@Module({
  imports: [WinstonModule.forRoot(LoggerFactory('ws-quran'))],
  exports: [WinstonModule],
  providers: [
    {
      provide: WINSTON_LOGGER,
      useFactory: LoggerFactory,
      inject: [],
    },
  ],
})
export class CommonModule {}
