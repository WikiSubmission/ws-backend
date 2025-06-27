import { HttpService } from '@nestjs/axios';
import { Injectable } from '@nestjs/common';
import { firstValueFrom } from 'rxjs';
import { LoggerService } from '@m/logger/logger.service';

@Injectable()
export class ApiRestaudioService {
  constructor(
    private readonly httpService: HttpService,
    private readonly logger: LoggerService,
  ) {}

  private readonly CDNChapterAudioAvailability =
    'https://raw.githubusercontent.com/islamic-network/cdn/master/info/cdn_surah_audio.json';
  public buildChapterAudioUrl({
    bitrate,
    edition,
    number,
  }: {
    bitrate: number | string;
    edition: string;
    number: number | string;
  }): string {
    return `https://cdn.islamic.network/quran/audio-surah/${bitrate}/${edition}/${number}.mp3`;
  }

  private readonly VerseByVerseAudioURLAvailability =
    ' https://api.alquran.cloud/v1/edition/format/audio';
  private readonly VerseByVerseSpecificRecitation =
    'https://api.alquran.cloud/v1/surah/1/';
  public buildVerseByVerseAudioUrl({
    bitrate,
    edition,
    number,
  }: {
    bitrate: number | string;
    edition: string;
    number: number | string;
  }): string {
    return `https://cdn.islamic.network/quran/audio/${bitrate}/${edition}/${number}.mp3`;
  }

  async fetchChapterAudioAvailability(): Promise<Record<string, any>> {
    try {
      const response = await firstValueFrom(
        this.httpService.get(this.CDNChapterAudioAvailability),
      );
      this.logger.log({ message: 'Fetched surah audio metadata' });
      return response.data as Record<string, any>;
    } catch (error) {
      this.logger.error({
        message: 'Failed to fetch surah audio JSON',
        error,
        context: 'ApiRestaudioService.fetchAudioMetadata',
      });
      const errorMessage =
        error instanceof Error ? error.message : 'Unknown error';
      throw new Error(`Failed to fetch surah audio JSON: ${errorMessage}`);
    }
  }

  async fetchVerseAudioAvailability(): Promise<Record<string, any>> {
    try {
      const response = await firstValueFrom(
        this.httpService.get(this.VerseByVerseAudioURLAvailability),
      );
      this.logger.log({ message: 'Fetched verse audio' });
      return response.data as Record<string, any>;
    } catch (error) {
      this.logger.error({
        message: 'Failed to fetch verse audio',
        error,
        context: 'ApiRestaudioService.fetchVerseAudioAvailability',
      });
      const errorMessage =
        error instanceof Error ? error.message : 'Unknown error';
      throw new Error(`Failed to fetch verse audio: ${errorMessage}`);
    }
  }

  fetchChapterAudio({
    chapter,
    edition,
    bitrate,
  }: {
    chapter: number;
    edition: string;
    bitrate: number;
  }): string {
    try {
      const url = this.buildChapterAudioUrl({
        bitrate,
        edition,
        number: chapter,
      });
      return url;
    } catch (error) {
      this.logger.error({
        message: 'Failed to fetch surah audio url',
        error,
        context: 'ApiRestaudioService.fetchChapterAudio',
      });
      const errorMessage =
        error instanceof Error ? error.message : 'Unknown error';
      throw new Error(`Failed to fetch surah audio url: ${errorMessage}`);
    }
  }
}
