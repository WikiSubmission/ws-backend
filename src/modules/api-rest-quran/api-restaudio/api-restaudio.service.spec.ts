import { Test, TestingModule } from '@nestjs/testing';
import { ApiRestaudioService } from './api-restaudio.service';
import { HttpService } from '@nestjs/axios';
import { LoggerService } from '@modules';
import { of, throwError } from 'rxjs';
import { AxiosResponse, AxiosHeaders } from 'axios';

describe('ApiRestaudioService', () => {
  let service: ApiRestaudioService;

  const mockLogger = {
    log: jest.fn(),
    error: jest.fn(),
  };

  const mockHttpService = {
    get: jest.fn(),
  };

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [
        ApiRestaudioService,
        { provide: LoggerService, useValue: mockLogger },
        { provide: HttpService, useValue: mockHttpService },
      ],
    }).compile();

    service = module.get<ApiRestaudioService>(ApiRestaudioService);
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it('should be defined', () => {
    expect(service).toBeDefined();
  });

  describe('buildChapterAudioUrl', () => {
    it('should return correct URL', () => {
      const url = service.buildChapterAudioUrl({
        bitrate: 128,
        edition: 'ar.alafasy',
        number: 1,
      });
      expect(url).toBe(
        'https://cdn.islamic.network/quran/audio-surah/128/ar.alafasy/1.mp3',
      );
    });
  });

  describe('buildVerseByVerseAudioUrl', () => {
    it('should return correct URL', () => {
      const url = service.buildVerseByVerseAudioUrl({
        bitrate: 64,
        edition: 'en.walk',
        number: 99,
      });
      expect(url).toBe(
        'https://cdn.islamic.network/quran/audio/64/en.walk/99.mp3',
      );
    });
  });

  describe('fetchChapterAudioAvailability', () => {
    it('should return audio metadata', async () => {
      const mockResponse: AxiosResponse = {
        data: { example: true },
        status: 200,
        statusText: 'OK',
        headers: {},
        config: {
          headers: AxiosHeaders.from({}),
        },
      };

      mockHttpService.get.mockReturnValue(of(mockResponse));
      const result = await service.fetchChapterAudioAvailability();

      expect(result).toEqual({ example: true });
      expect(mockLogger.log).toHaveBeenCalledWith({
        message: 'Fetched surah audio metadata',
      });
    });

    it('should throw error on failure', async () => {
      const mockError = new Error('Network error');
      mockHttpService.get.mockReturnValue(throwError(() => mockError));

      await expect(service.fetchChapterAudioAvailability()).rejects.toThrow(
        'Failed to fetch surah audio JSON: Network error',
      );

      expect(mockLogger.error).toHaveBeenCalledWith({
        message: 'Failed to fetch surah audio JSON',
        error: mockError,
        context: 'ApiRestaudioService.fetchAudioMetadata',
      });
    });
  });

  describe('fetchVerseAudioAvailability', () => {
    it('should return verse audio metadata', async () => {
      const mockResponse: AxiosResponse = {
        data: { success: true },
        status: 200,
        statusText: 'OK',
        headers: {},
        config: {
          headers: AxiosHeaders.from({}),
        },
      };

      mockHttpService.get.mockReturnValue(of(mockResponse));
      const result = await service.fetchVerseAudioAvailability();

      expect(result).toEqual({ success: true });
      expect(mockLogger.log).toHaveBeenCalledWith({
        message: 'Fetched verse audio',
      });
    });

    it('should throw error on failure', async () => {
      const mockError = new Error('Verse audio API error');
      mockHttpService.get.mockReturnValue(throwError(() => mockError));

      await expect(service.fetchVerseAudioAvailability()).rejects.toThrow(
        'Failed to fetch verse audio: Verse audio API error',
      );

      expect(mockLogger.error).toHaveBeenCalledWith({
        message: 'Failed to fetch verse audio',
        error: mockError,
        context: 'ApiRestaudioService.fetchVerseAudioAvailability',
      });
    });
  });

  describe('fetchChapterAudio', () => {
    it('should return a valid chapter audio URL', () => {
      const url = service.fetchChapterAudio({
        chapter: 2,
        edition: 'ar.husary',
        bitrate: 64,
      });

      expect(url).toBe(
        'https://cdn.islamic.network/quran/audio-surah/64/ar.husary/2.mp3',
      );
    });

    it('should handle error and log', () => {
      const brokenService = new ApiRestaudioService(
        mockHttpService as any,
        mockLogger as any,
      );

      jest
        .spyOn(brokenService, 'buildChapterAudioUrl')
        .mockImplementationOnce(() => {
          throw new Error('URL failure');
        });

      expect(() =>
        brokenService.fetchChapterAudio({
          chapter: 1,
          edition: 'bad.edition',
          bitrate: 128,
        }),
      ).toThrow('Failed to fetch surah audio url: URL failure');

      expect(mockLogger.error).toHaveBeenCalledWith(
        expect.objectContaining({
          message: 'Failed to fetch surah audio url',
          context: 'ApiRestaudioService.fetchChapterAudio',
        }),
      );
    });
  });
});
