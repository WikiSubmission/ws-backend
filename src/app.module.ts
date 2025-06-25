import { Module } from '@nestjs/common';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { PrismaModule } from './modules';
import { GatewayModule } from './modules/gateway/gateway.module';

@Module({
  imports: [PrismaModule, GatewayModule],
  controllers: [AppController],
  providers: [AppService],
})
export class AppModule {}
