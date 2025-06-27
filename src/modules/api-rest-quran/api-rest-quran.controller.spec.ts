import { Test, TestingModule } from '@nestjs/testing';
import { ApiRestQuranController } from './api-rest-quran.controller';
import { ApiRestQuranService } from './api-rest-quran.service';
import { LoggerService } from '../logger/logger.service';
import { ApiRestaudioService } from './api-restaudio/api-restaudio.service';
import { QuranQueryParamsDto } from './dto/quran-query.dto';

describe('ApiRestQuranController', () => {
  let controller: ApiRestQuranController;

  const mockUtilities = {
    getQuran: jest.fn(),
    getRandomVerse: jest.fn(),
    getRandomChapter: jest.fn(),
  };

  const mockLogger = {
    log: jest.fn(),
  };

  const mockAudio = {
    fetchChapterAudioAvailability: jest
      .fn()
      .mockResolvedValue({ available: true }),
    fetchVerseAudioAvailability: jest
      .fn()
      .mockResolvedValue({ available: true }),
    fetchChapterAudio: jest
      .fn()
      .mockReturnValue('https://example.com/audio.mp3'),
  };

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [ApiRestQuranController],
      providers: [
        { provide: ApiRestQuranService, useValue: mockUtilities },
        { provide: LoggerService, useValue: mockLogger },
        { provide: ApiRestaudioService, useValue: mockAudio },
      ],
    }).compile();

    controller = module.get<ApiRestQuranController>(ApiRestQuranController);
  });

  it('should be defined', () => {
    expect(controller).toBeDefined();
  });

  it('should call getQuran on service with query', async () => {
    const dto: QuranQueryParamsDto = { query: '1:1' };
    mockUtilities.getQuran.mockResolvedValue(['result']);

    const result = await controller.getQuran(dto);
    expect(mockUtilities.getQuran).toHaveBeenCalledWith(dto);
    expect(result).toEqual(['result']);
  });

  it('should return a random verse', async () => {
    mockUtilities.getRandomVerse.mockResolvedValue({ content: 'Verse' });

    const result = await controller.getRandomVerse();
    expect(mockUtilities.getRandomVerse).toHaveBeenCalled();
    expect(result).toEqual({ content: 'Verse' });
  });

  it('should return a random chapter', async () => {
    mockUtilities.getRandomChapter.mockResolvedValue({ name: 'Chapter' });

    const result = await controller.getRandomChapter();
    expect(mockUtilities.getRandomChapter).toHaveBeenCalled();
    expect(result).toEqual({ name: 'Chapter' });
  });

  it('should fetch chapter audio metadata', async () => {
    const result = await controller.fetchChapterAudioAvailability();
    expect(mockAudio.fetchChapterAudioAvailability).toHaveBeenCalled();
    expect(result).toEqual({ available: true });
  });

  it('should fetch verse audio metadata', async () => {
    const result = await controller.fetchVerseAudioAvailability();
    expect(mockAudio.fetchVerseAudioAvailability).toHaveBeenCalled();
    expect(result).toEqual({ available: true });
  });

  it('should return static chapter audio url', () => {
    const result = controller.fetchChapterAudio();
    expect(mockAudio.fetchChapterAudio).toHaveBeenCalledWith({
      chapter: 1,
      edition: 'ar.alafasy',
      bitrate: 128,
    });
    expect(result).toBe('https://example.com/audio.mp3');
  });
});
