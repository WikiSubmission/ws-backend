import { ApiProperty } from '@nestjs/swagger';
export class FileUploadResponseDto {
  @ApiProperty({
    description: 'Public URL of the uploaded file',
    example: 'https://your-bucket.s3.amazonaws.com/image-1640995200000.jpg',
  })
  url!: string;

  @ApiProperty({
    description: 'S3 key of the uploaded file',
    example: 'image-1640995200000.jpg',
  })
  key!: string;
}

export class MultipartInitResponseDto {
  @ApiProperty({
    description: 'Upload ID for the multipart upload session',
    example: 'ExampleUploadId123456789',
  })
  uploadId!: string;
}

export class CompleteMultipartResponseDto {
  @ApiProperty({
    description: 'Final URL of the completed multipart upload',
    example: 'https://your-bucket.s3.amazonaws.com/large-video.mp4',
  })
  url!: string;
}

export class UploadPartResponseDto {
  @ApiProperty({
    description: 'ETag of the uploaded part',
    example: '"1234567890abcdef"',
  })
  ETag!: string;

  @ApiProperty({
    description: 'Part number that was uploaded',
    example: 1,
  })
  partNumber!: number;
}
