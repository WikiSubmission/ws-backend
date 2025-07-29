import { ApiProperty } from '@nestjs/swagger';
import { Type } from 'class-transformer';
import { IsArray, IsNumber, IsString, ValidateNested } from 'class-validator';

export class MultipartInitDto {
  @ApiProperty({
    description: 'Name of the file to upload',
    example: 'large-video.mp4',
  })
  @IsString()
  fileName!: string;
}

export class PartUploadDto {
  @ApiProperty({
    description: 'ETag returned from S3 for the uploaded part',
    example: '"1234567890abcdef"',
  })
  @IsString()
  ETag!: string;

  @ApiProperty({
    description: 'Part number (starting from 1)',
    example: 1,
  })
  @IsNumber()
  partNumber!: number;
}

export class CompleteMultipartDto {
  @ApiProperty({
    description: 'Upload ID from the initial multipart upload',
    example: 'ExampleUploadId123456789',
  })
  @IsString()
  uploadId!: string;

  @ApiProperty({
    description: 'Name of the file being uploaded',
    example: 'large-video.mp4',
  })
  @IsString()
  fileName!: string;

  @ApiProperty({
    description: 'Array of uploaded parts with their ETags and part numbers',
    type: [PartUploadDto],
  })
  @IsArray()
  @ValidateNested({ each: true })
  @Type(() => PartUploadDto)
  parts!: PartUploadDto[];
}
