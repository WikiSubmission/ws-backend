import { Module } from '@nestjs/common';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { PrismaModule } from '@modules';
import { LoggerModule } from '@modules';
import { ApiRestQuranModule } from '@modules';
import { FilesModule } from './modules/files/files.module';

@Module({
  imports: [PrismaModule, LoggerModule, ApiRestQuranModule, FilesModule],
  controllers: [AppController],
  providers: [AppService],
})
export class AppModule {}
