import { createZodDto } from 'nestjs-zod';
import {
  SCHEMA_REQUEST,
  SCHEMA_CLIENT_PREFERENCES,
  SCHEMA_PAGINATION_CLIENT,
} from 'wikisubmission-sdk';
import z from 'zod';

// Create DTOs for request validation
export class RequestDto extends createZodDto(SCHEMA_REQUEST) {}
export class ClientPreferencesDto extends createZodDto(
  SCHEMA_CLIENT_PREFERENCES,
) {}
export class PaginationDto extends createZodDto(SCHEMA_PAGINATION_CLIENT) {}
export type ClientPreferencesType = z.infer<typeof SCHEMA_CLIENT_PREFERENCES>;
