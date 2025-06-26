import { Module } from '@nestjs/common';
import { ApiRestQuranService } from './api-rest-quran.service';
import { ApiRestQuranController } from './api-rest-quran.controller';
import { LoggerModule, PrismaModule } from '@modules';

@Module({
  imports: [PrismaModule, LoggerModule],
  providers: [ApiRestQuranService],
  controllers: [ApiRestQuranController],
})
export class ApiRestQuranModule {}
