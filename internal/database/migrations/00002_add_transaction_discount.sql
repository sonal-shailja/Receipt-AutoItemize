-- +goose Up
-- Receipt-level discount in minor units, always positive; subtracted when
-- reconciling line items against the grand total.
ALTER TABLE transactions ADD COLUMN discount_minor INTEGER;

-- +goose Down
ALTER TABLE transactions DROP COLUMN discount_minor;
