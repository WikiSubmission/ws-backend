import { Test, TestingModule } from '@nestjs/testing';
import { ApiRestQuranController } from './api-rest-quran.controller';

describe('ApiRestQuranController', () => {
  let controller: ApiRestQuranController;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [ApiRestQuranController],
    }).compile();

    controller = module.get<ApiRestQuranController>(ApiRestQuranController);
  });

  it('should be defined', () => {
    expect(controller).toBeDefined();
  });
});
