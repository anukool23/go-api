ALTER TABLE listings 
ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'ready' CHECK (
    status IN (
        'processing',
        'ready',
        'rejected'
    )
);

ALTER TABLE listings 
ALTER COLUMN status SET DEFAULT 'processing';

CREATE INDEX IF NOT EXISTS idx_listing_status on listings (status);