CREATE TABLE IF NOT EXISTS image_variants (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    image_id     UUID NOT NULL REFERENCES images(id) ON DELETE CASCADE,
    variant_type TEXT NOT NULL,
    file_path    TEXT NOT NULL,
    width        INT  NOT NULL,
    height       INT  NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_image_variants_image_id ON image_variants(image_id);
