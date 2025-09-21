import { createZodDto } from 'nestjs-zod';
import { SCHEMA_REQUEST } from 'wikisubmission-sdk';
import z from 'zod';

export class RequestDto extends createZodDto(SCHEMA_REQUEST) {}
export type RequestType = z.infer<typeof SCHEMA_REQUEST>;
