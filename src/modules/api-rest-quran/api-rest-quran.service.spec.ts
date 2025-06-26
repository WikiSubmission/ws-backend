import { Test, TestingModule } from '@nestjs/testing';
import { ApiRestQuranService } from './api-rest-quran.service';

describe('ApiRestQuranService', () => {
  let service: ApiRestQuranService;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [ApiRestQuranService],
    }).compile();

    service = module.get<ApiRestQuranService>(ApiRestQuranService);
  });

  it('should be defined', () => {
    expect(service).toBeDefined();
  });
});
