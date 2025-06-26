import { Module } from '@nestjs/common';
import { ApiRestQuranService } from './api-rest-quran.service';
import { ApiRestQuranController } from './api-rest-quran.controller';
import { LoggerModule, PrismaModule } from '@modules';
import { HttpModule } from '@nestjs/axios';
import { ApiRestaudioService } from './api-restaudio/api-restaudio.service';

@Module({
  imports: [PrismaModule, LoggerModule, HttpModule],
  providers: [ApiRestQuranService, ApiRestaudioService, ApiRestaudioService],
  controllers: [ApiRestQuranController],
})
export class ApiRestQuranModule {}
