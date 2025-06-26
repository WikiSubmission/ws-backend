import { ApiProperty } from '@nestjs/swagger';

export class VerseDto {
  @ApiProperty({ example: 1 })
  number!: number;
  @ApiProperty({ example: '' })
  subtitle!: string;
  @ApiProperty({
    example: 'In the name of GOD, Most Gracious, Most Merciful.*',
  })
  content!: string;
  @ApiProperty({
    example:
      '*1:1 The first verse in the Quran represents the foundation upon which a superhuman 19-based mathematical miracle is built. This important statement consists of 19 Arabic letters, and every word in it occurs in the whole Quran in multiples of 19 (see Appendices 1 & 29 for the details).',
  })
  footnote!: string;
}

export class ChapterDto {
  @ApiProperty({ example: 1 })
  number!: number;
  @ApiProperty({ example: 'The Opening' })
  name!: string;
  @ApiProperty({
    type: [VerseDto],
    example: [
      {
        number: 1,
        subtitle: 'In the name of God',
        content: 'In the name of GOD, Most Gracious, Most Merciful.*',
        footnote:
          '*1:1 The first verse in the Quran represents the foundation upon which a superhuman 19-based mathematical miracle is built. This important statement consists of 19 Arabic letters, and every word in it occurs in the whole Quran in multiples of 19 (see Appendices 1 & 29 for the details).',
      },
      {
        number: 2,
        subtitle: '',
        content: 'Praise be to God, Lord of the universe.',
        footnote: '',
      },
    ],
  })
  verses!: VerseDto[];
}

export class QuranResponseChapter {
  @ApiProperty({ enum: ['chapter'], example: 'chapter' })
  type!: 'chapter';
  @ApiProperty({ type: ChapterDto })
  data!: ChapterDto;
}

export class QuranResponseVerse {
  @ApiProperty({ enum: ['verse'], example: 'verse' })
  type!: 'verse';
  @ApiProperty({ example: 1 })
  chapterNum!: number;
  @ApiProperty({ example: 'The Key' })
  chapterTitle!: string;
  @ApiProperty({ type: VerseDto })
  data!: VerseDto;
}

export class QuranResponseVerses {
  @ApiProperty({ enum: ['verses'], example: 'verses' })
  type!: 'verses';
  @ApiProperty({ example: 1 })
  chapterNum!: number;
  @ApiProperty({ example: 'The Key' })
  chapterTitle!: string;
  @ApiProperty({
    type: [VerseDto],
    example: [
      {
        number: 1,
        subtitle: '',
        content: 'In the name of GOD, Most Gracious, Most Merciful.*',
        footnote:
          '*1:1 The first verse in the Quran represents the foundation upon which a superhuman 19-based mathematical miracle is built. This important statement consists of 19 Arabic letters, and every word in it occurs in the whole Quran in multiples of 19 (see Appendices 1 & 29 for the details).',
      },
      {
        number: 2,
        subtitle: '',
        content: 'Praise be to GOD, Lord of the universe.',
        footnote: '',
      },
    ],
  })
  data!: VerseDto[];
}
