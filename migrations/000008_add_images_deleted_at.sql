-- Add soft-delete support to images table
ALTER TABLE images ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_images_deleted_at ON images (deleted_at);

ALTER TABLE images DROP COLUMN IF EXISTS filename;

ALTER TABLE images MODIFY filename TEXT NULL;