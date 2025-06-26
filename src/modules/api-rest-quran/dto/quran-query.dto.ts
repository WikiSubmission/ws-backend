import { IsString, Matches } from 'class-validator';
import { regex } from '../regex.constant';
import { ApiProperty } from '@nestjs/swagger';

export class QuranQueryDto {
  // eslint-disable-next-line @typescript-eslint/no-unsafe-call
  // @Matches(regex, {
  //   message: 'Query must be a valid format like 1, 1:2, 2:4-6, 5:1,6',
  // })
  @ApiProperty({
    description: 'Query string like 1:1, 1:2-4',
    example: '1:1-7',
  })
  query!: string;
}
