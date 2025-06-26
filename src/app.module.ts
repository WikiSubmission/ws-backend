import { Module } from '@nestjs/common';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { PrismaModule } from '@modules';
import { LoggerModule } from '@modules';
import { ApiRestQuranModule } from '@modules';

@Module({
  imports: [PrismaModule, LoggerModule, ApiRestQuranModule],
  controllers: [AppController],
  providers: [AppService],
})
export class AppModule {}
