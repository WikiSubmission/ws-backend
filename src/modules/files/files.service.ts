import { Injectable, Logger } from '@nestjs/common';
import {
  S3Client,
  PutObjectCommand,
  CreateMultipartUploadCommand,
  UploadPartCommand,
  CompleteMultipartUploadCommand,
  DeleteObjectCommand,
  PutObjectCommandInput,
  CreateMultipartUploadCommandInput,
  UploadPartCommandInput,
  CompleteMultipartUploadCommandInput,
  DeleteObjectCommandInput,
} from '@aws-sdk/client-s3';
import * as path from 'path';

const REQUIRED_ENV_VARS = [
  'AWS_ACCESS_KEY_ID',
  'AWS_SECRET_ACCESS_KEY',
  'AWS_REGION',
  'AWS_BUCKET_NAME',
];

for (const varName of REQUIRED_ENV_VARS) {
  if (!process.env[varName]) {
    throw new Error(`Missing required environment variable: ${varName}`);
  }
}

Injectable();
export class FilesService {
  private readonly logger = new Logger(FilesService.name);
  private readonly s3Client: S3Client;
  private readonly bucketName = process.env.AWS_BUCKET_NAME!;

  constructor() {
    this.s3Client = new S3Client({
      credentials: {
        accessKeyId: process.env.AWS_ACCESS_KEY_ID!,
        secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY!,
      },
      region: process.env.AWS_REGION!,
    });

    this.logger.log({
      message: 'FilesService initialized with S3 client',
      context: 'constructor',
      region: process.env.AWS_REGION,
      bucket: this.bucketName,
    });
  }

  async uploadFile(
    file: Express.Multer.File,
  ): Promise<{ url: string; key: string }> {
    this.logger.log({
      message: 'Starting file upload',
      context: 'uploadFile',
      originalName: file.originalname,
      size: file.size,
      mimetype: file.mimetype,
    });

    const fileExtension = path.extname(file.originalname);
    const fileName = `${path.basename(file.originalname, fileExtension)}-${Date.now()}${fileExtension}`;

    const params: PutObjectCommandInput = {
      Bucket: this.bucketName,
      Key: fileName,
      Body: file.buffer,
      ContentType: file.mimetype,
    };

    try {
      const command = new PutObjectCommand(params);
      const result = await this.s3Client.send(command);

      const url = `https://${this.bucketName}.s3.${process.env.AWS_REGION}.amazonaws.com/${fileName}`;

      this.logger.log({
        message: 'File uploaded successfully',
        context: 'uploadFile',
        key: fileName,
        url,
        etag: result.ETag,
      });

      return {
        url,
        key: fileName,
      };
    } catch (error) {
      this.logger.error({
        message: 'Failed to upload file',
        context: 'uploadFile',
        error: error instanceof Error ? error.message : 'Unknown error',
        fileName,
      });
      throw new Error('Failed to upload file');
    }
  }

  async initialMultipartUpload(fileName: string): Promise<string> {
    this.logger.log({
      message: 'Initiating multipart upload',
      context: 'initialMultipartUpload',
      fileName,
    });

    const params: CreateMultipartUploadCommandInput = {
      Bucket: this.bucketName,
      Key: fileName,
    };

    try {
      const command = new CreateMultipartUploadCommand(params);
      const result = await this.s3Client.send(command);

      if (!result.UploadId) {
        this.logger.error({
          message: 'No upload ID received from S3',
          context: 'initialMultipartUpload',
          fileName,
        });
        throw new Error('Failed to get upload ID from S3');
      }

      this.logger.log({
        message: 'Multipart upload initiated successfully',
        context: 'initialMultipartUpload',
        fileName,
        uploadId: result.UploadId,
      });

      return result.UploadId;
    } catch (error) {
      this.logger.error({
        message: 'Failed to initiate multipart upload',
        context: 'initialMultipartUpload',
        error: error instanceof Error ? error.message : 'Unknown error',
        fileName,
      });
      throw new Error('Failed to initiate multipart upload');
    }
  }

  async uploadPart(
    uploadId: string,
    partNumber: number,
    body: Buffer,
    fileName: string,
  ): Promise<{ ETag: string; partNumber: number }> {
    this.logger.log({
      message: 'Uploading part',
      context: 'uploadPart',
      fileName,
      uploadId,
      partNumber,
      size: body.length,
    });

    const params: UploadPartCommandInput = {
      Bucket: this.bucketName,
      Key: fileName,
      PartNumber: partNumber,
      UploadId: uploadId,
      Body: body,
    };

    try {
      const command = new UploadPartCommand(params);
      const result = await this.s3Client.send(command);

      if (!result.ETag) {
        this.logger.error({
          message: 'No ETag received from S3',
          context: 'uploadPart',
          fileName,
          uploadId,
          partNumber,
        });
        throw new Error('Failed to get ETag from S3');
      }

      this.logger.log({
        message: 'Part uploaded successfully',
        context: 'uploadPart',
        fileName,
        uploadId,
        partNumber,
        etag: result.ETag,
      });

      return {
        ETag: result.ETag,
        partNumber,
      };
    } catch (error) {
      this.logger.error({
        message: 'Failed to upload part',
        context: 'uploadPart',
        error: error instanceof Error ? error.message : 'Unknown error',
        fileName,
        uploadId,
        partNumber,
      });
      throw new Error('Failed to upload part');
    }
  }

  async completeMultipartUpload(
    uploadId: string,
    fileName: string,
    parts: { PartNumber: number; ETag: string }[],
  ): Promise<string> {
    this.logger.log({
      message: 'Completing multipart upload',
      context: 'completeMultipartUpload',
      fileName,
      uploadId,
      partsCount: parts.length,
    });

    const params: CompleteMultipartUploadCommandInput = {
      Bucket: this.bucketName,
      Key: fileName,
      UploadId: uploadId,
      MultipartUpload: {
        Parts: parts,
      },
    };

    try {
      const command = new CompleteMultipartUploadCommand(params);
      const result = await this.s3Client.send(command);

      if (!result.Location) {
        this.logger.error({
          message: 'No location received from S3',
          context: 'completeMultipartUpload',
          fileName,
          uploadId,
        });
        throw new Error('Failed to get location from S3');
      }

      this.logger.log({
        message: 'Multipart upload completed successfully',
        context: 'completeMultipartUpload',
        fileName,
        uploadId,
        location: result.Location,
        etag: result.ETag,
      });

      return result.Location;
    } catch (error) {
      this.logger.error({
        message: 'Failed to complete multipart upload',
        context: 'completeMultipartUpload',
        error: error instanceof Error ? error.message : 'Unknown error',
        fileName,
        uploadId,
      });
      throw new Error('Failed to complete multipart upload');
    }
  }

  async deleteFile(key: string): Promise<void> {
    this.logger.log({
      message: 'Starting file deletion',
      context: 'deleteFile',
      key,
    });

    const params: DeleteObjectCommandInput = {
      Bucket: this.bucketName,
      Key: key,
    };

    try {
      const command = new DeleteObjectCommand(params);
      await this.s3Client.send(command);

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
      throw new Error('Failed to delete file');
    }
  }
}
