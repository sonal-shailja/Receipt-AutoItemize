-- +goose Up
CREATE TABLE receipts (
    id           TEXT PRIMARY KEY,
    file_name    TEXT    NOT NULL,
    content_type TEXT    NOT NULL,
    file_path    TEXT    NOT NULL,
    size_bytes   INTEGER NOT NULL,
    sha256       TEXT    NOT NULL,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

CREATE TABLE ocr_results (
    id         TEXT PRIMARY KEY,
    receipt_id TEXT    NOT NULL REFERENCES receipts (id),
    provider   TEXT    NOT NULL,
    status     TEXT    NOT NULL,
    raw_text   TEXT,
    error      TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX idx_ocr_results_receipt ON ocr_results (receipt_id);

CREATE TABLE transactions (
    id                TEXT PRIMARY KEY,
    receipt_id        TEXT    NOT NULL UNIQUE REFERENCES receipts (id),
    ocr_result_id     TEXT    NOT NULL REFERENCES ocr_results (id),
    merchant          TEXT,
    txn_date          TEXT,
    currency          TEXT,
    subtotal_minor    INTEGER,
    grand_total_minor INTEGER,
    itemize_status    TEXT    NOT NULL,
    itemize_reason    TEXT,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);

CREATE TABLE taxes (
    id             TEXT PRIMARY KEY,
    transaction_id TEXT    NOT NULL REFERENCES transactions (id),
    position       INTEGER NOT NULL,
    name           TEXT    NOT NULL,
    rate_bps       INTEGER NOT NULL,
    amount_minor   INTEGER NOT NULL,
    inclusive      INTEGER NOT NULL,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    deleted_at     INTEGER
);
CREATE INDEX idx_taxes_transaction ON taxes (transaction_id, deleted_at);

CREATE TABLE line_items (
    id               TEXT PRIMARY KEY,
    transaction_id   TEXT    NOT NULL REFERENCES transactions (id),
    position         INTEGER NOT NULL,
    description      TEXT    NOT NULL,
    amount_minor     INTEGER NOT NULL,
    quantity         INTEGER,
    tax_amount_minor INTEGER,
    source           TEXT    NOT NULL,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    deleted_at       INTEGER
);
CREATE INDEX idx_line_items_transaction ON line_items (transaction_id, deleted_at);

-- +goose Down
DROP TABLE line_items;
DROP TABLE taxes;
DROP TABLE transactions;
DROP TABLE ocr_results;
DROP TABLE receipts;
