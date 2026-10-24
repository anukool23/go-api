DROP INDEX IF EXISTS  idx_listing_status;

ALTER TABLE listings DROP COLUMN IF EXISTS status;