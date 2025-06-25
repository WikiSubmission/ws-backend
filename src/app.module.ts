import { Module } from '@nestjs/common';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { PrismaModule } from './modules';
import { LoggerModule } from './modules/logger/logger.module';

@Module({
  imports: [PrismaModule, LoggerModule],
  controllers: [AppController],
  providers: [AppService],
})
export class AppModule {}
