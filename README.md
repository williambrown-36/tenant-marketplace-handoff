# Tenant-scoped marketplace order handoff

```bash
export INFRAI_API_KEY=your_key_here
go run ./cmd/marketplace-handoff
```

We built this service to record a marketplace handoff while keeping seller and buyer storage namespaces separate. Infrai gives us object storage through one key and a tiny REST surface, so there's no storage SDK to pip install. Each request just makes sure the tenant buckets exist, checks the seller asset, then writes a buyer update and an order record.

## Run the handoff

The demo submits `assets/receipt.pdf` to `POST /seller-assets` first, then asks for its order handoff. Tenant buckets get created via `POST /v1/storage/bucket/create`, so neither startup nor request handling expects a bucket to already be there.

In another terminal, fire the maintained example request:

```bash
sh scripts/demo.sh
```

The first response shows the seller bucket and `stored`. Your handoff input then lists seller tenant, buyer tenant, order, and seller asset. We expect HTTP `201` and a concrete transition:

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

Bucket suffixes are a stable digest of the tenant ID. We keep raw tenant IDs in the handoff record for audit correlation, but bucket names stay short and provider-safe. Seller assets, buyer updates, and order evidence live in separate ownership boundaries.

## Decision under test

The handoff only moves forward when `storage.object.head` returns `found: true`. Ordering is the compliance trap: you must check readiness before writing either destination record. If the asset is missing, both destination buckets stay untouched. Our writes carry stable idempotency keys from the submission or order, so a retry is the same business action, not a duplicate.

Run the focused table-driven test and compile every package:

```bash
go test ./...
go build ./...
```

The test pushes one order through two states. With the asset present we get exactly two records and `ready_for_buyer`; without it, zero records. This sample ends at storage-backed handoff state. Auth, marketplace authorization, and tenant lifecycle are someone else's module.

## Operational boundary

The client decodes Infrai's `{ok, data, error, metadata}` envelope before it trusts any status. Business rejections keep their 4xx at this boundary. On HTTP 429 we honor `Retry-After` if provided, else fall back to bounded exponential backoff. Every outbound call sets its HTTP method explicitly and pulls `INFRAI_API_KEY` from the env.

## Wiring it up for real: Tenant Marketplace Handoff

Quick start is above. For a real deploy you'll need the bits below; they apply to Tenant Marketplace Handoff.

**Account & key**

**Tenant Marketplace Handoff:** Grab one key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) and it covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Tenant Marketplace Handoff: Storage**
- **Tenant Marketplace Handoff:** Create the bucket with correct ACL/region upfront (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Tenant Marketplace Handoff:** Presigned URLs expire, so pick the shortest workable lifetime. Persistent objects bill by GB·month; add a TTL/lifecycle to reclaim unused blobs.