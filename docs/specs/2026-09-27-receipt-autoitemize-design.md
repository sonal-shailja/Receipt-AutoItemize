# Receipt upload, taxes, auto-itemize — design

Date: 2026-09-27 · Status: approved

## Problem

A user uploads a receipt. The system must create exactly one transaction from it, store its taxes as their
own records, and auto-itemize it into line items the user can later edit. When the items do not reconcile
with the receipt total, the transaction is kept and flagged for review — never balanced with an invented line.

Success bar: the three fixtures in `fixtures/task-a/` produce the fields in `gold.json`, and a reviewer can
run the service with one command and exercise every endpoint with the README curls.

## Goals / non-goals

Goals: the five endpoints in the brief plus `GET /health`; a correct relational data model; replayable
extraction from stored OCR text; a 409 on user edits that break reconciliation.

Non-goals: real OCR vendor, auth, UI, async processing, confidence scores, duplicate-receipt rejection,
production file storage, invoicing/inventory/travel.

## Components

| Unit | Purpose | Depends on |
|---|---|---|
| `ocr.Provider` | `Extract(file) → raw text`. `stub` impl: `text/plain` upload → its bytes; otherwise look up `fixtures/task-a/<basename>.txt`. | filesystem |
| `extraction` | Pure `Parse(text) → Parsed{header, taxes, items}`. No I/O. | — |
| `reconcile` | Pure `Check(total, taxes, items) → Result{status, reason, diff}`. Shared by process, itemize and PATCH. | — |
| `receipt` service | Upload (store file, sha256), Process, Itemize, UpdateItems, Get. Owns transactions/idempotency. | repos, ocr, extraction, reconcile |
| entities + repos | `receipts`, `ocr_results`, `transactions`, `taxes`, `line_items` | GORM/SQLite |

A real vendor provider (e.g. a vision LLM reading a key from an env var) would slot in behind `ocr.Provider`;
the provider name is recorded on each OCR result.

## Receipt vs transaction

- **Receipt**: the uploaded file. Never changed after upload.
- **OCR result**: the text read from the receipt. Each `process` call adds a new one, so older reads are kept.
- **Transaction**: the expense created from the receipt (merchant, total, taxes, line items). One per
  receipt. The user can edit its line items; `/itemize` re-reads them from the stored OCR text.

## Data model

```sql
CREATE TABLE receipts (
  id            TEXT PRIMARY KEY,          -- rcpt_<ulid>
  file_name     TEXT NOT NULL,
  content_type  TEXT NOT NULL,             -- text/plain | image/* | application/pdf
  file_path     TEXT NOT NULL,             -- data/uploads/<id><ext>
  size_bytes    INTEGER NOT NULL,
  sha256        TEXT NOT NULL,
  created_at    INTEGER NOT NULL           -- unix ms; row is never updated
);

CREATE TABLE ocr_results (
  id          TEXT PRIMARY KEY,            -- ocr_<ulid>
  receipt_id  TEXT NOT NULL REFERENCES receipts(id),
  provider    TEXT NOT NULL,               -- stub
  status      TEXT NOT NULL,               -- SUCCEEDED | FAILED
  raw_text    TEXT,                        -- full OCR output; source for re-itemize
  error       TEXT,                        -- set when FAILED
  created_at  INTEGER NOT NULL
);
CREATE INDEX idx_ocr_results_receipt ON ocr_results(receipt_id);

CREATE TABLE transactions (
  id                 TEXT PRIMARY KEY,     -- txn_<ulid>
  receipt_id         TEXT NOT NULL UNIQUE REFERENCES receipts(id),
  ocr_result_id      TEXT NOT NULL REFERENCES ocr_results(id),  -- the OCR run this was built from
  merchant           TEXT,
  txn_date           TEXT,                 -- YYYY-MM-DD
  currency           TEXT,
  subtotal_minor     INTEGER,              -- NULL when not printed
  grand_total_minor  INTEGER,              -- NULL only when FAILED
  discount_minor     INTEGER,              -- receipt-level discount, positive; added by 00002
  itemize_status     TEXT NOT NULL,        -- COMPLETE | NEEDS_REVIEW | FAILED
  itemize_reason     TEXT,                 -- total_mismatch | no_line_items | total_not_found
  created_at         INTEGER NOT NULL,
  updated_at         INTEGER NOT NULL
);

CREATE TABLE taxes (
  id              TEXT PRIMARY KEY,        -- tax_<ulid>
  transaction_id  TEXT NOT NULL REFERENCES transactions(id),
  name            TEXT NOT NULL,
  rate_bps        INTEGER NOT NULL,        -- 1900 = 19%; API renders 0.19
  amount_minor    INTEGER NOT NULL,
  inclusive       INTEGER NOT NULL,        -- 1 = already contained in the total
  position        INTEGER NOT NULL,
  created_at      INTEGER NOT NULL,
  deleted_at      INTEGER                  -- soft delete; NULL = live
);

CREATE TABLE line_items (
  id                TEXT PRIMARY KEY,      -- li_<ulid>
  transaction_id    TEXT NOT NULL REFERENCES transactions(id),
  position          INTEGER NOT NULL,
  description       TEXT NOT NULL,
  amount_minor      INTEGER NOT NULL,
  quantity          INTEGER,
  tax_amount_minor  INTEGER,               -- only when the receipt prints it; never derived
  source            TEXT NOT NULL,         -- AUTO | USER
  created_at        INTEGER NOT NULL,
  updated_at        INTEGER NOT NULL,
  deleted_at        INTEGER                -- soft delete; NULL = live
);
CREATE INDEX idx_taxes_txn ON taxes(transaction_id, deleted_at);
CREATE INDEX idx_line_items_txn ON line_items(transaction_id, deleted_at);
```

Taxes and line items are never hard-deleted: replacing them soft-deletes the live rows and inserts new ones,
so every earlier version (e.g. the OCR spelling before a user correction) stays queryable. All reads filter
`deleted_at IS NULL`. Item ids therefore change on every replacement.

Money is `int64` minor units end to end (2 decimal places assumed); OCR decimals are parsed exactly
(string → cents), never via float math. Taxes are receipt-level rows exactly as printed; mixed rates become
multiple rows. Per-item tax is not allocated from the receipt tax.

## Parsing rules (text fixtures)

- `MERCHANT:` / `DATE:` / `CURRENCY:` header lines.
- Summary lines are not items: `TOTAL <amount>` → grand total, `Subtotal <amount>` → subtotal.
- `VAT 19% <amount>` → exclusive tax (added on top of the subtotal); `incl. VAT 19% <amount>` → inclusive tax
  (already inside the total). Name, rate and amount all come from the printed line.
- Any other line ending in an amount → line item. Lines with no amount (a service description such as
  "Trip fare", wrapped descriptions, headers) and `(…)` notes are ignored — turning them into items would
  invent data.

## Reconciliation and status

An internal check, not an endpoint, run by process, itemize and PATCH: do the items plus the taxes add up to
what was paid? `Σ items + Σ exclusive taxes − discount == grand_total`, tolerance 1 minor unit. Inclusive taxes are
already inside the total.

| Condition | Status | Reason |
|---|---|---|
| Total not found | `FAILED` | `total_not_found` |
| No items | `NEEDS_REVIEW` | `no_line_items` |
| Sum ≠ total | `NEEDS_REVIEW` | `total_mismatch` |
| Sum = total | `COMPLETE` | — |

The grand total is never changed and no balancing line is ever added. Expected fixture results:
clean → `COMPLETE`; tax-only → `NEEDS_REVIEW`, `line_items: []`; mismatch → `NEEDS_REVIEW` with its two items.

## Data flow

1. `POST /receipts` — store file under the configured upload dir, persist receipt, return `receipt_id`.
2. `POST /receipts/{id}/process` — run OCR and insert an `ocr_results` row. On OCR failure the row is
   `FAILED`, the call returns **422** `ocr_failed`, and any existing transaction is untouched. On success:
   parse → reconcile → in one DB transaction upsert the transaction (keyed by receipt_id, pointed at the new
   OCR result) and replace its taxes and items. Re-processing updates, never duplicates.
3. `GET /transactions/{id}` — header + live taxes + live line_items + itemize_status/reason.
4. `POST /transactions/{id}/itemize` — re-parse the raw text of the transaction's OCR result; replace line items
   and status only. This discards user edits by design (they remain as soft-deleted rows).
5. `PATCH /transactions/{id}/items` — body is the full new item list (covers add, remove, edit, merge, split).
   Reconciles against the stored total and taxes → replace items as `source=USER`, status `COMPLETE`.
   Otherwise **409** with `{expected_total, items_total, tax_total, difference}` and nothing persisted.
   Descriptions are free text; only amounts affect reconciliation.

## Errors

Error body: `{"error":{"code","message","request_id"}}`, extended with a `details` map for the 409
mismatch. Unknown ids → 404; bad input (missing file, empty item description, negative amount) → 400.

## Testing

- Golden test: each fixture through parse + reconcile equals the gold values.
- Unit tests for parser edge lines and reconciler table.
- Service tests: re-process keeps one transaction and adds an OCR result; itemize replaces items only; replaced
  items are soft-deleted; PATCH mismatch → 409 and no write.
- README curls double as the manual end-to-end check.

## Assumptions

- Gold item amounts are net; the fixtures' item lines already are, so no gross/net conversion is needed.
- Uploads are small; no size limit beyond a sane cap (10 MB).
- One currency per receipt; no FX.
- The brief's fixtures are copied into the repo at `fixtures/task-a/` so tests, curls and the stub OCR lookup work from a fresh clone.
