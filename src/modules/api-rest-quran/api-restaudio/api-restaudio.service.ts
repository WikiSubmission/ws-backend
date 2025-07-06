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

  private readonly verseCounts: Record<number, number> = {
    1: 7,
    2: 286,
    3: 200,
    4: 176,
    5: 120,
    6: 165,
    7: 206,
    8: 75,
    9: 127,
    10: 109,
    11: 123,
    12: 111,
    13: 43,
    14: 52,
    15: 99,
    16: 128,
    17: 111,
    18: 110,
    19: 98,
    20: 135,
    21: 112,
    22: 78,
    23: 118,
    24: 64,
    25: 77,
    26: 227,
    27: 93,
    28: 88,
    29: 69,
    30: 60,
    31: 34,
    32: 30,
    33: 73,
    34: 54,
    35: 45,
    36: 83,
    37: 182,
    38: 88,
    39: 75,
    40: 85,
    41: 54,
    42: 53,
    43: 89,
    44: 59,
    45: 37,
    46: 35,
    47: 38,
    48: 29,
    49: 18,
    50: 45,
    51: 60,
    52: 49,
    53: 62,
    54: 55,
    55: 78,
    56: 96,
    57: 29,
    58: 22,
    59: 24,
    60: 13,
    61: 14,
    62: 11,
    63: 11,
    64: 18,
    65: 12,
    66: 12,
    67: 30,
    68: 52,
    69: 52,
    70: 44,
    71: 28,
    72: 28,
    73: 20,
    74: 56,
    75: 40,
    76: 31,
    77: 50,
    78: 40,
    79: 46,
    80: 42,
    81: 29,
    82: 19,
    83: 36,
    84: 25,
    85: 22,
    86: 17,
    87: 19,
    88: 26,
    89: 30,
    90: 20,
    91: 15,
    92: 21,
    93: 11,
    94: 8,
    95: 8,
    96: 19,
    97: 5,
    98: 8,
    99: 8,
    100: 11,
    101: 11,
    102: 8,
    103: 3,
    104: 9,
    105: 5,
    106: 4,
    107: 7,
    108: 3,
    109: 6,
    110: 3,
    111: 5,
    112: 4,
    113: 5,
    114: 6,
  };

  private getTraditionalVerseIndex({
    chapterNumber,
    verseNumber,
  }: {
    chapterNumber: number;
    verseNumber: number;
  }): number {
    if (!this.verseCounts[chapterNumber]) {
      throw new Error(`Invalid chapter number: ${chapterNumber}`);
    }
    if (chapterNumber === 9 && (verseNumber === 128 || verseNumber === 129)) {
      throw new Error('Invalid verse number: 9:128 or 9:129');
    }
    if (verseNumber > this.verseCounts[chapterNumber]) {
      if (
        !(chapterNumber === 9 && (verseNumber === 128 || verseNumber === 129))
      )
        throw new Error(
          `Invalid verse number: ${chapterNumber}:${verseNumber}`,
        );
    }

    let index = 0;
    for (let i = 0; i < chapterNumber; i++) {
      index += this.verseCounts[i];
      if (i === 9) {
        index += 2;
      }
    }

    index += verseNumber;

    return index;
  }
  private readonly CDNChapterAudioAvailability =
    'https://raw.githubusercontent.com/islamic-network/cdn/master/info/cdn_surah_audio.json';
  private readonly VerseByVerseAudioURLAvailability =
    ' https://api.alquran.cloud/v1/edition/format/audio';
  private readonly VerseByVerseSpecificRecitation =
    'https://api.alquran.cloud/v1/surah/1/';

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
  public buildVerseByVerseAudioUrl({
    bitrate,
    edition,
    chapterNumber,
    verseNumber,
  }: {
    bitrate: number | string;
    edition: string;
    chapterNumber: number;
    verseNumber: number;
  }): string {
    const number = this.getTraditionalVerseIndex({
      chapterNumber,
      verseNumber,
    });
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
