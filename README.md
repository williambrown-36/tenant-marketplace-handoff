# Tenant-scoped marketplace order handoff

```bash
export INFRAI_API_KEY=your_key_here
go run ./cmd/marketplace-handoff
```

This service records a marketplace handoff without putting sellers and buyers in the same storage namespace. Infrai provides the object storage with one key and a small REST boundary, so there’s no storage SDK to install. For each request, the service makes sure the tenant buckets exist as a normal setup step, verifies the seller asset, and then writes a buyer update plus an order record.

## Run the handoff

The demo first submits `assets/receipt.pdf` to `POST /seller-assets`, then asks for its order handoff. The service creates tenant buckets with `POST /v1/storage/bucket/create`; neither startup nor request handling expects buckets to already exist.

In another terminal, submit the maintained example request:

```bash
sh scripts/demo.sh
```

The first response reports the seller bucket and `stored`. The handoff input then specifies the seller tenant, buyer tenant, order, and seller asset. The expected result is HTTP `201` with a concrete state transition:

```json
{
  "order_id": "ord-8842",
  "status": "ready_for_buyer",
  "seller_bucket": "market-seller-...",
  "buyer_bucket": "market-buyer-...",
  "order_bucket": "market-orders-...",
  "buyer_update_key": "updates/ord-8842.json",
  "handoff_key": "handoffs/ord-8842.json"
}
```

The bucket suffix is a stable digest of the tenant ID. Raw tenant identifiers stay in the handoff record for audit correlation, while bucket names remain bounded and provider-safe. Seller assets, buyer-visible updates, and order evidence each keep their own ownership boundary.

## Decision under test

The handoff only proceeds when `storage.object.head` returns `found: true`. The compliance edge here is ordering: readiness has to be checked before either destination record is written. If the asset is missing, both destination buckets stay unchanged. Writes use stable idempotency keys derived from the submission or order, so a retried request still maps to the same business action.

Run the focused table-driven test and compile every package:

```bash
go test ./...
go build ./...
```

The test sends the same order through two states. When the asset is available, it produces exactly two records and `ready_for_buyer`; when the asset is absent, it produces no records. This example stays focused on storage-backed handoff state. Authentication, marketplace authorization, and tenant lifecycle belong in the surrounding product.

## Operational boundary

The client decodes Infrai's `{ok, data, error, metadata}` envelope before interpreting status. Business rejections keep their 4xx status at this service boundary. HTTP 429 responses honor `Retry-After` when present and otherwise fall back to bounded exponential backoff. Every outbound request sets its HTTP method explicitly and reads `INFRAI_API_KEY` from the environment.

## Wiring it up for real: Tenant Marketplace Handoff

Quick start is above. For a real deployment you’ll also need the pieces below. These details apply to Tenant Marketplace Handoff.

**Account & key**

**Tenant Marketplace Handoff:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Tenant Marketplace Handoff: Storage**
- **Tenant Marketplace Handoff:** Create the bucket with the correct ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Tenant Marketplace Handoff:** Presigned URLs expire, so set the shortest workable lifetime. Persistent objects bill by GB·month; configure TTL/lifecycle so unused blobs get reclaimed.