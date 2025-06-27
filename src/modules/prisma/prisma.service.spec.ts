import { Test, TestingModule } from '@nestjs/testing';
import { PrismaService } from './prisma.service';
import { LoggerService } from '../logger/logger.service';

describe('PrismaService', () => {
  let service: PrismaService;

  beforeEach(async () => {
    const mockLogger = {
      log: jest.fn(),
    };

    const module: TestingModule = await Test.createTestingModule({
      providers: [
        PrismaService,
        {
          provide: LoggerService,
          useValue: mockLogger,
        },
      ],
    }).compile();

    service = module.get<PrismaService>(PrismaService);

    // Optionally, mock Prisma connect/disconnect
    jest.spyOn(service, '$connect').mockResolvedValue(undefined);
    jest.spyOn(service, '$disconnect').mockResolvedValue(undefined);
  });

  it('should be defined', () => {
    expect(service).toBeDefined();
  });

  it('should call $connect and log on init', async () => {
    const connectSpy = jest.spyOn(service, '$connect');
    await service.onModuleInit();
    expect(connectSpy).toHaveBeenCalled();
  });

  it('should call $disconnect and log on destroy', async () => {
    const disconnectSpy = jest.spyOn(service, '$disconnect');
    await service.onModuleDestroy();
    expect(disconnectSpy).toHaveBeenCalled();
  });
});
