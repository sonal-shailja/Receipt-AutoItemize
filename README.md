# Receipt auto-itemize

A small HTTP API for receipts. A user uploads a receipt, and processing it creates **one transaction**, stores its **taxes as separate rows**, and **auto-itemizes** it into line items. Re-itemize replays the stored OCR text. When a user edit no longer matches the receipt total, the API returns `409`. When line items do not add up, the transaction is kept and marked `NEEDS_REVIEW`. The code never adds a balancing line and never changes the total.

> **OCR is stubbed.** No OCR or LLM vendor is called, and no API key is needed.
> - A `text/plain` upload is treated as OCR output and used as-is.
> - An image or PDF upload is matched by base name to a fixture: `receipt-clean.png` reads `fixtures/task-a/receipt-clean.txt`.
>
> Providers sit behind `ocr.Provider`, so a real vendor can replace the stub. See [ARCHITECTURE.md](ARCHITECTURE.md).

## Run

```bash
make run            # or: go run ./cmd/api
```

- Needs only Go. Go 1.21 or newer automatically downloads the pinned toolchain (go 1.25).
- REST listens on `:8080`. gRPC listens on `127.0.0.1:8082` and is used by the REST gateway.
- On first boot the app creates `data/app.db` (SQLite), runs the migrations, and creates `data/uploads/`.
- `make clean` resets everything.

Without Go:

```bash
docker build -t receipt-api . && docker run --rm -p 8080:8080 receipt-api
```

Tests: `make test`. Tests run against real, migrated, temp-file SQLite databases, and one of them checks all three fixtures against `gold.json`.

## Endpoints

All examples run from the repo root against a fresh server. Responses are real, with IDs shortened.

### `GET /health`

```bash
curl -s localhost:8080/health
```
```json
{"status":"SERVING","dependencies":[{"name":"database","status":"SERVING","error":""}]}
```

### `POST /receipts`: upload (multipart field `file`)

```bash
curl -s -F "file=@fixtures/task-a/receipt-clean.txt" localhost:8080/receipts
```
```json
{"receipt_id":"rcpt_01M3…","file_name":"receipt-clean.txt","content_type":"text/plain","size_bytes":252,"sha256":"4ca0916c…"}
```

Accepted types are `text/plain`, `image/*` and `application/pdf`, up to 10 MB. Anything else returns `400`.

To set the variable used by the next steps:

```bash
RID=$(curl -s -F "file=@fixtures/task-a/receipt-clean.txt" localhost:8080/receipts | jq -r .receipt_id)
```

### `POST /receipts/{id}/process`: OCR, extraction, auto-itemize

```bash
curl -s -X POST localhost:8080/receipts/$RID/process
```
```json
{
  "id": "txn_01M3…", "receipt_id": "rcpt_01M3…", "ocr_result_id": "ocr_01M3…",
  "merchant": "Cafe Mitte", "date": "2026-03-12", "currency": "EUR",
  "subtotal": 15, "grand_total": 17.85,
  "taxes": [{"id": "tax_01M3…", "name": "VAT", "rate": 0.19, "amount": 2.85, "inclusive": false}],
  "line_items": [
    {"id": "li_01M3…", "description": "Espresso",      "amount": 3.5, "source": "AUTO"},
    {"id": "li_01M3…", "description": "Sandwich",      "amount": 8.9, "source": "AUTO"},
    {"id": "li_01M3…", "description": "Mineral water", "amount": 2.6, "source": "AUTO"}
  ],
  "itemize_status": "COMPLETE", "itemize_reason": ""
}
```

Processing the same receipt again updates the same transaction rather than creating a second one, and records a new OCR result. When OCR finds no text, the call returns `422 ocr_failed`, keeps a `FAILED` OCR result, and leaves any existing transaction unchanged.

To set the variable used by the next steps:

```bash
TID=$(curl -s -X POST localhost:8080/receipts/$RID/process | jq -r .id)
```

### `GET /transactions/{id}`

```bash
curl -s localhost:8080/transactions/$TID
```

Returns the same shape as above.

### `PATCH /transactions/{id}/items`: user override (edit, merge, split)

The body is the **complete new list** of items. This example merges Espresso and water into one item:

```bash
curl -s -X PATCH localhost:8080/transactions/$TID/items -H 'Content-Type: application/json' \
  -d '{"items":[{"description":"Drinks","amount":6.10},{"description":"Sandwich","amount":8.90}]}'
```
```json
{"itemize_status":"COMPLETE","line_items":[{"description":"Drinks","amount":6.1,"source":"USER"},{"description":"Sandwich","amount":8.9,"source":"USER"}], "…": "…"}
```

If the items no longer add up, the API returns `409`, saves nothing, and does not adjust any total:

```bash
curl -s -X PATCH localhost:8080/transactions/$TID/items -H 'Content-Type: application/json' \
  -d '{"items":[{"description":"Espresso","amount":3.50}]}'
```
```json
{"error":{"code":"items_total_mismatch","message":"line items plus taxes minus discount total 6.35 but the receipt total is 17.85",
 "details":{"expected_total":"17.85","items_total":"3.50","tax_total":"2.85","discount_total":"0.00","difference":"11.50","reason":"total_mismatch"},
 "request_id":"req_01M3…"}}
```

Each item also accepts an optional `quantity` and `tax_amount`. Amounts must be zero or positive, with at most two decimals, and no larger than 10 trillion. JSON request bodies are capped at 1 MB.

### `POST /transactions/{id}/itemize`: re-run auto-itemize from stored OCR

```bash
curl -s -X POST localhost:8080/transactions/$TID/itemize
```

This returns the same transaction ID with the `AUTO` items re-parsed from the stored OCR text. It replaces only the line items. The header and taxes stay unchanged, and no second transaction is created. It deliberately replaces user edits; the replaced rows are soft-deleted, so they remain queryable.

### The three fixtures

```bash
for f in receipt-clean receipt-tax-only receipt-mismatch; do
  rid=$(curl -s -F "file=@fixtures/task-a/$f.txt" localhost:8080/receipts | jq -r .receipt_id)
  curl -s -X POST localhost:8080/receipts/$rid/process | jq -c '{merchant, grand_total, itemize_status, itemize_reason, taxes, items: [.line_items[] | {description, amount}]}'
done
```

| Fixture | Result |
|---|---|
| `receipt-clean` | `COMPLETE`: 3 items (15.00) + VAT 2.85 exclusive = 17.85 |
| `receipt-tax-only` | `NEEDS_REVIEW` / `no_line_items`: `line_items: []`; VAT 3.83 is `inclusive: true` (already inside 24.00); "Trip fare" has no amount, so no item is created |
| `receipt-mismatch` | `NEEDS_REVIEW` / `total_mismatch`: 2 items (10.00) + VAT 1.90 = 11.90, but the total stays **18.50** and no balancing line is added |

## Data model

| Table | Holds | Lifecycle |
|---|---|---|
| `receipts` | the uploaded file: path, type, size, sha256 | insert-only |
| `ocr_results` | one row per `process` run: provider, `SUCCEEDED`/`FAILED`, **raw OCR text**, error | insert-only; re-itemize replays `raw_text` |
| `transactions` | merchant, date, currency, subtotal, grand total, receipt-level `discount` (positive, nullable), `itemize_status` + reason; unique `receipt_id`; `ocr_result_id` it was built from | one per receipt |
| `taxes` | `name`, `rate` (basis points; the API shows `0.19`), `amount`, `inclusive` | replaced as a set; old rows soft-deleted |
| `line_items` | `description`, `amount`, optional `quantity` / `tax_amount`, `source` `AUTO`/`USER` | replaced as a set; old rows soft-deleted |

Money is stored as `int64` minor units (cents for EUR, paise for INR), in columns ending in `_minor`. OCR amounts are parsed from strings exactly, and floats appear only in the JSON. Per-item `tax_amount` is stored only when it is supplied; it is never derived from the receipt tax. The schema is in [`internal/database/migrations/`](internal/database/migrations/).

How receipt lines are read:
- `TOTAL`, `Subtotal`, `VAT 19%` / `incl. VAT 19%` and `Discount` lines are summary lines, not items. A trailing colon is accepted (`TOTAL: 17.85`).
- A `Discount` line is stored as the receipt-level discount, always as a positive amount, whether it is printed as `1.00` or `-1.00`.
- Item amounts are positive. Any other line printed with a minus is ignored rather than guessed at.
- Lines with no amount (such as "Trip fare") and `(…)` notes are ignored.

## Reconciliation

```
Σ line items + Σ exclusive taxes − discount == grand total   (±0.01)
```

Inclusive taxes ("incl. VAT") are already inside the total, so they are not added again. The same rule is applied by `process`, `itemize` and `PATCH`.

| Condition | `itemize_status` | `itemize_reason` |
|---|---|---|
| No total on the receipt | `FAILED` | `total_not_found` |
| No line items | `NEEDS_REVIEW` | `no_line_items` |
| Sum ≠ total | `NEEDS_REVIEW` | `total_mismatch` |
| Sum = total | `COMPLETE` | |

## Decisions and trade-offs

- **SQLite with a pure-Go driver.** A reviewer can run the service with one command and no database server. GORM repositories and goose migrations keep a switch to MySQL or Postgres small.
- **gRPC with a REST gateway.** Four endpoints are gRPC methods with REST routes defined in the `.proto` files. `POST /receipts` is a plain HTTP handler on the same mux, because grpc-gateway cannot handle multipart uploads. Both paths call the same service.
- **Evidence is never rewritten.** Receipts and OCR results are insert-only. Taxes and items are soft-deleted when replaced, so earlier versions (such as the OCR spelling before a user correction) are kept.
- **PATCH takes the full item list.** Edit, merge, split, add and remove all use one request shape, and there is no per-item matching. As a result, item IDs change on every PATCH.
- **Concurrency.** SQLite runs on one connection, so writes are serialized. `itemize` and `PATCH` read the transaction inside their DB transaction and write only the status columns, so a concurrent re-process cannot be overwritten with stale data.

## Layout

```
cmd/api                      entrypoint
config/default.toml          settings (APP_* env vars override, e.g. APP_SERVER_HTTP)
proto/ → rpc/                API definitions and generated code (committed)
internal/boot                wiring, gRPC + REST listeners, error envelope
internal/services/receipt    service (upload/process/itemize/override), gRPC handler, upload handler
internal/extraction          OCR text → header, taxes, items (pure)
internal/reconcile           the status rule (pure)
internal/ocr                 Provider interface + stub
internal/entities            receipt (+ OCR results), transaction (+ taxes, items) repositories
internal/database            SQLite connection, embedded migrations, transactions
pkg/money  pkg/filestore  pkg/id
fixtures/task-a              the brief's fixtures and gold.json
```
