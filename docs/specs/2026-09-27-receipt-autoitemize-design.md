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

## Chosen approach

Mirror the `slice-lld-server` skeleton: gRPC services exposed as REST through grpc-gateway, protos generated
with buf (generated code committed), GORM repositories, goose SQL migrations, viper TOML config, `slog`,
prefixed ULID ids, typed app errors mapped to HTTP status.

Deviations from slice, each for the one-command-run requirement:

- **SQLite (pure-Go driver, no CGO)** instead of MySQL; the DB file is created and migrated on boot.
- **No Redis** — nothing needs a cache or lock.
- **`POST /receipts` is a plain HTTP handler** registered on the gateway mux, because grpc-gateway cannot
  bind multipart bodies. It calls the same service layer as the gRPC methods.

Routes use the brief's exact paths (no `/v1` prefix).

## Components

| Unit | Purpose | Depends on |
|---|---|---|
| `ocr.Provider` | `Extract(file) → raw text`. `stub` impl: `text/plain` upload → its bytes; otherwise look up `fixtures/task-a/<basename>.txt`. | filesystem |
| `extraction` | Pure `Parse(text) → Parsed{header, taxes, items}`. No I/O. | — |
| `reconcile` | Pure `Check(total, taxes, items) → Result{status, reason, diff}`. Shared by process, itemize and PATCH. | — |
| `receipt` service | Upload (store file, sha256), Process, Itemize, UpdateItems, Get. Owns transactions/idempotency. | repos, ocr, extraction, reconcile |
| entities + repos | `receipts`, `transactions`, `taxes`, `line_items` | GORM/SQLite |

A real vendor provider (e.g. a vision LLM reading a key from an env var) would slot in behind `ocr.Provider`;
the provider name is recorded on the receipt.

## Data model

- `receipts`: id, file_path, file_name, content_type, sha256, ocr_provider, raw_ocr_text, processed_at.
- `transactions`: id, **unique** receipt_id, merchant, date, currency, grand_total, itemize_status, itemize_reason.
- `taxes`: id, transaction_id, name, rate, amount, inclusive.
- `line_items`: id, transaction_id, position, description, amount, optional quantity and tax_amount.

Money is `int64` minor units end to end; OCR decimals are parsed exactly (string → cents), never via float math.

## Parsing rules (text fixtures)

- `MERCHANT:` / `DATE:` / `CURRENCY:` header lines.
- `TOTAL <amount>` → grand total. `Subtotal` is read but not stored as an item.
- `VAT 19% <amount>` → exclusive tax; `incl. VAT 19% <amount>` → inclusive tax. Rate is stored as a fraction (0.19).
- Any other line ending in an amount → line item. Lines without an amount ("Trip fare") and `(…)` notes are ignored.

## Reconciliation and status

`Σ items + Σ exclusive taxes == grand_total`, tolerance 1 minor unit. Inclusive taxes are already inside the total.

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
2. `POST /receipts/{id}/process` — OCR → store raw text → parse → reconcile → in one DB transaction upsert the
   transaction (keyed by receipt_id) and replace its taxes and items. Re-processing updates, never duplicates.
3. `GET /transactions/{id}` — header + taxes + line_items + itemize_status/reason.
4. `POST /transactions/{id}/itemize` — re-parse the **stored** raw OCR text; replace line items and status only.
5. `PATCH /transactions/{id}/items` — body is the full new item list (covers edit, merge, split). Reconciles →
   replace items, status `COMPLETE`. Otherwise **409** with `{expected_total, items_total, tax_total,
   difference}` and nothing persisted.

## Errors

Slice's envelope: `{"error":{"code","message","request_id"}}`, extended with a `details` map for the 409
mismatch. Unknown ids → 404; bad input (missing file, empty item description, negative amount) → 400.

## Testing

- Golden test: each fixture through parse + reconcile equals the gold values.
- Unit tests for parser edge lines and reconciler table.
- Service tests: re-process keeps one transaction; itemize replaces items only; PATCH mismatch → 409 and no write.
- README curls double as the manual end-to-end check.

## Assumptions

- Gold item amounts are net; the fixtures' item lines already are, so no gross/net conversion is needed.
- Uploads are small; no size limit beyond a sane cap (10 MB).
- One currency per receipt; no FX.
- The brief's fixtures are copied into the repo at `fixtures/task-a/` so tests, curls and the stub OCR lookup work from a fresh clone.
