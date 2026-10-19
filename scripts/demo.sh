#!/bin/sh
set -eu

curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"seller_tenant":"seller_204","asset_key":"assets/receipt.pdf","data_base64":"c2V0dGxlZCBtYXJrZXRwbGFjZSByZWNlaXB0Cg==","submission_id":"submission-8842"}' \
  http://localhost:8080/seller-assets

curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"seller_tenant":"seller_204","buyer_tenant":"buyer_731","order_id":"ord-8842","asset_key":"assets/receipt.pdf"}' \
  http://localhost:8080/handoffs
