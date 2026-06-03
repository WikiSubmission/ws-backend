-- Rename the data-export storage columns from the legacy Drive naming to the
-- AWS S3 / CloudFront naming. Migrations run on every startup, so each rename is
-- guarded to stay idempotent (RENAME COLUMN has no IF EXISTS form).
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'user_data_exports' AND column_name = 'drive_file_id'
  ) THEN
    ALTER TABLE user_data_exports RENAME COLUMN drive_file_id TO object_key;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'user_data_exports' AND column_name = 'drive_url'
  ) THEN
    ALTER TABLE user_data_exports RENAME COLUMN drive_url TO download_url;
  END IF;
END $$;
