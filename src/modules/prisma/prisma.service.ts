// prisma.service.ts
import { Injectable, OnModuleInit, OnModuleDestroy } from '@nestjs/common';
import { PrismaClient } from '@prisma/client';
import { LoggerService } from '../logger/logger.service';

@Injectable()
export class PrismaService
  extends PrismaClient
  implements OnModuleInit, OnModuleDestroy
{
  constructor(private readonly logger: LoggerService) {
    super();
  }

  async onModuleInit() {
    await this.$connect();
    this.logger.log({
      level: 'info',
      message: 'PrismaService connected',
      context: 'PrismaService',
    });
  }

  async onModuleDestroy() {
    await this.$disconnect();
    this.logger.log({
      level: 'info',
      message: 'PrismaService disconnected',
      context: 'PrismaService',
    });
  }
}
