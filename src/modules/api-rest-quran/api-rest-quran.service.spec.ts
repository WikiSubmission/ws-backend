import { Test, TestingModule } from '@nestjs/testing';
import { ApiRestQuranService } from './api-rest-quran.service';
import { PrismaService } from '../prisma/prisma.service';
import { LoggerService } from '../logger/logger.service';
import { ApiRestaudioService } from './api-restaudio/api-restaudio.service';

describe('ApiRestQuranService', () => {
  let service: ApiRestQuranService;

  const mockPrisma = {
    chapter: {
      findFirstOrThrow: jest.fn(),
    },
    verse: {
      findFirstOrThrow: jest.fn(),
      findMany: jest.fn(),
    },
  };

  const mockLogger = {
    log: jest.fn(),
    error: jest.fn(),
  };

  const mockAudio = {
    buildChapterAudioUrl: jest
      .fn()
      .mockReturnValue('https://fake-audio.com/chapter.mp3'),
    buildVerseByVerseAudioUrl: jest
      .fn()
      .mockReturnValue('https://fake-audio.com/verse.mp3'),
  };

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [
        ApiRestQuranService,
        { provide: PrismaService, useValue: mockPrisma },
        { provide: LoggerService, useValue: mockLogger },
        { provide: ApiRestaudioService, useValue: mockAudio },
      ],
    }).compile();

    service = module.get<ApiRestQuranService>(ApiRestQuranService);
  });

  it('should be defined', () => {
    expect(service).toBeDefined();
  });

  it('getRandomChapter should return a mocked chapter', async () => {
    mockPrisma.chapter.findFirstOrThrow.mockResolvedValueOnce({
      number: 1,
      verses: [{ number: 1, content: 'Sample verse' }],
    });

    const chapter = await service.getRandomChapter();
    expect(chapter).toBeDefined();
    expect(mockPrisma.chapter.findFirstOrThrow).toHaveBeenCalled();
  });

  it('getRandomVerse should return a mocked verse', async () => {
    mockPrisma.chapter.findFirstOrThrow.mockResolvedValueOnce({
      number: 1,
      verses: [{ number: 1, content: 'Verse 1' }],
    });

    const verse = await service.getRandomVerse();
    expect(verse).toBeDefined();
    expect(verse.content).toBe('Verse 1');
  });

  it('parseQuranQuery should parse chapter input', () => {
    const result = (service as any).parseQuranQuery('2');
    expect(result).toEqual([{ chapterNum: 2, type: 'chapter' }]);
  });

  it('parseQuranQuery should parse verse input', () => {
    const result = (service as any).parseQuranQuery('2:5');
    expect(result).toEqual([{ chapterNum: 2, verse: 5, type: 'verse' }]);
  });

  it('parseQuranQuery should parse range input', () => {
    const result = (service as any).parseQuranQuery('2:1-5');
    expect(result).toEqual([
      { chapterNum: 2, start: 1, end: 5, type: 'range' },
    ]);
  });
});
