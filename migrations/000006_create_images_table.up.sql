CREATE TABLE IF NOT EXISTS images  (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    listing_id UUID NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    object_key TEXT NOT NULL UNIQUE,
    position SMALLINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_images_listing_id ON images (listing_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_images_listing_position ON images (listing_id, position);