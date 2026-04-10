-- +goose Up
CREATE INDEX IF NOT EXISTS idx_catalog_item_info_hash ON catalog_item (info_hash) WHERE info_hash <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_catalog_item_info_hash;
