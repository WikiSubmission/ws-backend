import {
  Controller,
  Post,
  Delete,
  UseInterceptors,
  UploadedFile,
  Body,
  Param,
  HttpCode,
  HttpStatus,
  BadRequestException,
  Logger,
} from '@nestjs/common';
import { FileInterceptor } from '@nestjs/platform-express';
import {
  ApiTags,
  ApiOperation,
  ApiResponse,
  ApiConsumes,
  ApiBody,
  ApiParam,
} from '@nestjs/swagger';
import { FilesService } from './files.service';
import {
  CompleteMultipartResponseDto,
  FileUploadResponseDto,
  MultipartInitResponseDto,
  UploadPartResponseDto,
} from './dto/response.dto';
import { CompleteMultipartDto, MultipartInitDto } from './dto/request.dto';

@ApiTags('Files')
@Controller('files')
export class FilesController {
  private readonly logger = new Logger(FilesController.name);

  constructor(private readonly filesService: FilesService) {}

  @Post('upload')
  @ApiOperation({
    summary: 'Upload a single file',
    description: 'Upload a file directly to S3. Suitable for files up to 5GB.',
  })
  @ApiConsumes('multipart/form-data')
  @ApiBody({
    description: 'File to upload',
    schema: {
      type: 'object',
      properties: {
        file: {
          type: 'string',
          format: 'binary',
          description: 'The file to upload',
        },
      },
      required: ['file'],
    },
  })
  @ApiResponse({
    status: 201,
    description: 'File uploaded successfully',
    type: FileUploadResponseDto,
  })
  @ApiResponse({
    status: 400,
    description: 'Bad request - no file provided or invalid file',
  })
  @ApiResponse({
    status: 500,
    description: 'Internal server error - upload failed',
  })
  @UseInterceptors(FileInterceptor('file'))
  async uploadFile(
    @UploadedFile() file: Express.Multer.File,
  ): Promise<FileUploadResponseDto> {
    this.logger.log({
      message: 'File upload request received',
      context: 'uploadFile',
      hasFile: !!file,
      originalName: file?.originalname,
      size: file?.size,
    });

    if (!file) {
      this.logger.warn({
        message: 'File upload request without file',
        context: 'uploadFile',
      });
      throw new BadRequestException('No file provided');
    }

    try {
      const result = await this.filesService.uploadFile(file);

      this.logger.log({
        message: 'File upload completed successfully',
        context: 'uploadFile',
        key: result.key,
        url: result.url,
      });

      return result;
    } catch (error) {
      this.logger.error({
        message: 'File upload failed',
        context: 'uploadFile',
        error: error instanceof Error ? error.message : 'Unknown error',
        originalName: file.originalname,
      });
      throw error;
    }
  }

  @Post('multipart/init')
  @ApiOperation({
    summary: 'Initialize multipart upload',
    description:
      'Start a multipart upload session for large files (>100MB recommended)',
  })
  @ApiResponse({
    status: 201,
    description: 'Multipart upload initialized successfully',
    type: MultipartInitResponseDto,
  })
  @ApiResponse({
    status: 400,
    description: 'Bad request - invalid filename',
  })
  @ApiResponse({
    status: 500,
    description: 'Internal server error - failed to initialize upload',
  })
  async initMultipartUpload(
    @Body() body: MultipartInitDto,
  ): Promise<MultipartInitResponseDto> {
    this.logger.log({
      message: 'Multipart upload initialization request',
      context: 'initMultipartUpload',
      fileName: body.fileName,
    });

    try {
      const uploadId = await this.filesService.initialMultipartUpload(
        body.fileName,
      );

      this.logger.log({
        message: 'Multipart upload initialized successfully',
        context: 'initMultipartUpload',
        fileName: body.fileName,
        uploadId,
      });

      return { uploadId };
    } catch (error) {
      this.logger.error({
        message: 'Failed to initialize multipart upload',
        context: 'initMultipartUpload',
        error: error instanceof Error ? error.message : 'Unknown error',
        fileName: body.fileName,
      });
      throw error;
    }
  }

  @Post('multipart/:uploadId/:partNumber/:fileName')
  @ApiOperation({
    summary: 'Upload a part for multipart upload',
    description:
      'Upload a single part of a multipart upload. Parts must be at least 5MB (except the last part).',
  })
  @ApiParam({
    name: 'uploadId',
    description: 'Upload ID from the multipart initialization',
    example: 'ExampleUploadId123456789',
  })
  @ApiParam({
    name: 'partNumber',
    description: 'Part number (starting from 1)',
    example: '1',
  })
  @ApiParam({
    name: 'fileName',
    description: 'Name of the file being uploaded',
    example: 'large-video.mp4',
  })
  @ApiConsumes('multipart/form-data')
  @ApiBody({
    description: 'File part to upload',
    schema: {
      type: 'object',
      properties: {
        file: {
          type: 'string',
          format: 'binary',
          description: 'The file part to upload',
        },
      },
      required: ['file'],
    },
  })
  @ApiResponse({
    status: 201,
    description: 'Part uploaded successfully',
    type: UploadPartResponseDto,
  })
  @ApiResponse({
    status: 400,
    description: 'Bad request - no file part provided or invalid parameters',
  })
  @ApiResponse({
    status: 500,
    description: 'Internal server error - part upload failed',
  })
  @UseInterceptors(FileInterceptor('file'))
  async uploadPart(
    @Param('uploadId') uploadId: string,
    @Param('partNumber') partNumber: string,
    @Param('fileName') fileName: string,
    @UploadedFile() file: Express.Multer.File,
  ): Promise<UploadPartResponseDto> {
    this.logger.log({
      message: 'Part upload request received',
      context: 'uploadPart',
      uploadId,
      partNumber,
      fileName,
      hasFile: !!file,
      fileSize: file?.size,
    });

    if (!file) {
      this.logger.warn({
        message: 'Part upload request without file',
        context: 'uploadPart',
        uploadId,
        partNumber,
        fileName,
      });
      throw new BadRequestException('No file part provided');
    }

    const partNum = parseInt(partNumber, 10);
    if (isNaN(partNum) || partNum < 1) {
      this.logger.warn({
        message: 'Invalid part number provided',
        context: 'uploadPart',
        partNumber,
        uploadId,
        fileName,
      });
      throw new BadRequestException('Invalid part number');
    }

    try {
      const result = await this.filesService.uploadPart(
        uploadId,
        partNum,
        file.buffer,
        fileName,
      );

      this.logger.log({
        message: 'Part uploaded successfully',
        context: 'uploadPart',
        uploadId,
        partNumber: partNum,
        fileName,
        etag: result.ETag,
      });

      return {
        ETag: result.ETag || '',
        partNumber: result.partNumber,
      };
    } catch (error) {
      this.logger.error({
        message: 'Part upload failed',
        context: 'uploadPart',
        error: error instanceof Error ? error.message : 'Unknown error',
        uploadId,
        partNumber: partNum,
        fileName,
      });
      throw error;
    }
  }

  @Post('multipart/complete')
  @ApiOperation({
    summary: 'Complete multipart upload',
    description: 'Finalize a multipart upload by combining all uploaded parts',
  })
  @ApiResponse({
    status: 201,
    description: 'Multipart upload completed successfully',
    type: CompleteMultipartResponseDto,
  })
  @ApiResponse({
    status: 400,
    description: 'Bad request - invalid upload data or missing parts',
  })
  @ApiResponse({
    status: 500,
    description: 'Internal server error - failed to complete upload',
  })
  async completeMultipartUpload(
    @Body() body: CompleteMultipartDto,
  ): Promise<CompleteMultipartResponseDto> {
    this.logger.log({
      message: 'Complete multipart upload request',
      context: 'completeMultipartUpload',
      uploadId: body.uploadId,
      fileName: body.fileName,
      partsCount: body.parts.length,
    });

    try {
      const parts = body.parts.map((part) => ({
        PartNumber: part.partNumber,
        ETag: part.ETag,
      }));

      const url = await this.filesService.completeMultipartUpload(
        body.uploadId,
        body.fileName,
        parts,
      );

      this.logger.log({
        message: 'Multipart upload completed successfully',
        context: 'completeMultipartUpload',
        uploadId: body.uploadId,
        fileName: body.fileName,
        url,
      });

      return { url };
    } catch (error) {
      this.logger.error({
        message: 'Failed to complete multipart upload',
        context: 'completeMultipartUpload',
        error: error instanceof Error ? error.message : 'Unknown error',
        uploadId: body.uploadId,
        fileName: body.fileName,
      });
      throw error;
    }
  }

  @Delete(':key')
  @ApiOperation({
    summary: 'Delete a file',
    description: 'Delete a file from S3 using its key',
  })
  @ApiParam({
    name: 'key',
    description: 'S3 key of the file to delete',
    example: 'image-1640995200000.jpg',
  })
  @ApiResponse({
    status: 204,
    description: 'File deleted successfully',
  })
  @ApiResponse({
    status: 400,
    description: 'Bad request - invalid file key',
  })
  @ApiResponse({
    status: 500,
    description: 'Internal server error - failed to delete file',
  })
  @HttpCode(HttpStatus.NO_CONTENT)
  async deleteFile(@Param('key') key: string): Promise<void> {
    this.logger.log({
      message: 'File deletion request',
      context: 'deleteFile',
      key,
    });

    if (!key) {
      this.logger.warn({
        message: 'File deletion request without key',
        context: 'deleteFile',
      });
      throw new BadRequestException('File key is required');
    }

    try {
      await this.filesService.deleteFile(key);

      this.logger.log({
        message: 'File deleted successfully',
        context: 'deleteFile',
        key,
      });
    } catch (error) {
      this.logger.error({
        message: 'Failed to delete file',
        context: 'deleteFile',
        error: error instanceof Error ? error.message : 'Unknown error',
        key,
      });
      throw error;
    }
  }
}
