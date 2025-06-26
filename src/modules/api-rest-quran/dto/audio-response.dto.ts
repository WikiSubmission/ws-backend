import { ApiProperty } from '@nestjs/swagger';

export class AudioResponseDto {
  @ApiProperty({ example: 'ar.abdulazizazzahrani' })
  identifier!: string;
  @ApiProperty({ example: 'ar' })
  language!: 'ar' | 'zh' | 'ru' | 'fr' | 'ur' | 'fa';
  @ApiProperty({ example: 'Abdul Bari Mohammed' })
  name!: string;
  @ApiProperty({ example: 'Abdul Bari Mohammed' })
  englishName!: string;
  @ApiProperty({ example: 'audio' })
  format!: 'audio';
  @ApiProperty({ example: 'surahbysurah' })
  type!: 'surahbysurah';
  @ApiProperty({ example: '128' })
  bitrate!: '192' | '128' | '64' | '48' | '40' | '32';
}

class VerseRecitationDetails {
  @ApiProperty({ example: 'ar.abdulazizazzahrani' })
  identifier!: string;
  @ApiProperty({ example: 'ar' })
  language!: 'ar' | 'zh' | 'ru' | 'fr' | 'ur' | 'fa';
  @ApiProperty({ example: 'Abdul Bari Mohammed' })
  name!: string;
  @ApiProperty({ example: 'Abdul Bari Mohammed' })
  englishName!: string;
  @ApiProperty({ example: 'audio' })
  format!: 'audio';
  @ApiProperty({ example: 'versebysurah' })
  type!: 'versebysurah' | 'translation';
  @ApiProperty({ example: null })
  direction!: null;
}

export class VerseByVerseAudioAvailabilityResponseDto {
  @ApiProperty({ example: 200 })
  code!: number;
  @ApiProperty({ example: 'OK' })
  status!: string;

  @ApiProperty({
    type: [VerseRecitationDetails],
    description: 'List of available verse recitations',
  })
  data!: VerseRecitationDetails[];
}
