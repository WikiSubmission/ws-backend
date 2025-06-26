import { Test, TestingModule } from '@nestjs/testing';
import { ApiRestaudioService } from './api-restaudio.service';

describe('ApiRestaudioService', () => {
  let service: ApiRestaudioService;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [ApiRestaudioService],
    }).compile();

    service = module.get<ApiRestaudioService>(ApiRestaudioService);
  });

  it('should be defined', () => {
    expect(service).toBeDefined();
  });
});
