package marketplace

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var tenantIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,62}$`)

var ErrAssetNotReady = errors.New("seller asset is not ready for handoff")

type Storage interface {
	EnsureBucket(context.Context, string) error
	ObjectHead(context.Context, string, string) (HeadResult, error)
	ObjectPut(context.Context, string, string, string, string) error
}

type HandoffService struct {
	Storage Storage
}

type HandoffRequest struct {
	SellerTenant string `json:"seller_tenant"`
	BuyerTenant  string `json:"buyer_tenant"`
	OrderID      string `json:"order_id"`
	AssetKey     string `json:"asset_key"`
}

type SellerAssetRequest struct {
	SellerTenant string `json:"seller_tenant"`
	AssetKey     string `json:"asset_key"`
	DataBase64   string `json:"data_base64"`
	SubmissionID string `json:"submission_id"`
}

type SellerAssetResult struct {
	SellerBucket string `json:"seller_bucket"`
	AssetKey     string `json:"asset_key"`
	Status       string `json:"status"`
}

type HandoffResult struct {
	OrderID        string `json:"order_id"`
	Status         string `json:"status"`
	SellerBucket   string `json:"seller_bucket"`
	BuyerBucket    string `json:"buyer_bucket"`
	OrderBucket    string `json:"order_bucket"`
	BuyerUpdateKey string `json:"buyer_update_key"`
	HandoffKey     string `json:"handoff_key"`
}

type handoffRecord struct {
	OrderID      string `json:"order_id"`
	SellerTenant string `json:"seller_tenant"`
	BuyerTenant  string `json:"buyer_tenant"`
	AssetBucket  string `json:"asset_bucket"`
	AssetKey     string `json:"asset_key"`
	Status       string `json:"status"`
}

func (s HandoffService) StoreSellerAsset(ctx context.Context, input SellerAssetRequest) (SellerAssetResult, error) {
	if !tenantIDPattern.MatchString(input.SellerTenant) {
		return SellerAssetResult{}, errors.New("seller tenant ID must use lowercase letters, digits, underscore, or hyphen")
	}
	if strings.TrimSpace(input.AssetKey) == "" || input.DataBase64 == "" || strings.TrimSpace(input.SubmissionID) == "" {
		return SellerAssetResult{}, errors.New("asset_key, data_base64, and submission_id are required")
	}
	bucket := tenantBucket("seller", input.SellerTenant)
	if err := s.Storage.EnsureBucket(ctx, bucket); err != nil {
		return SellerAssetResult{}, fmt.Errorf("prepare seller bucket: %w", err)
	}
	if err := s.Storage.ObjectPut(ctx, bucket, input.AssetKey, input.DataBase64, "seller-asset-"+input.SubmissionID); err != nil {
		return SellerAssetResult{}, fmt.Errorf("write seller asset: %w", err)
	}
	return SellerAssetResult{SellerBucket: bucket, AssetKey: input.AssetKey, Status: "stored"}, nil
}

func (s HandoffService) Handoff(ctx context.Context, input HandoffRequest) (HandoffResult, error) {
	if !tenantIDPattern.MatchString(input.SellerTenant) || !tenantIDPattern.MatchString(input.BuyerTenant) {
		return HandoffResult{}, errors.New("tenant IDs must use lowercase letters, digits, underscore, or hyphen")
	}
	if strings.TrimSpace(input.OrderID) == "" || strings.TrimSpace(input.AssetKey) == "" {
		return HandoffResult{}, errors.New("order_id and asset_key are required")
	}

	sellerBucket := tenantBucket("seller", input.SellerTenant)
	buyerBucket := tenantBucket("buyer", input.BuyerTenant)
	orderBucket := tenantBucket("orders", input.SellerTenant)
	for _, bucket := range []string{sellerBucket, buyerBucket, orderBucket} {
		if err := s.Storage.EnsureBucket(ctx, bucket); err != nil {
			return HandoffResult{}, fmt.Errorf("prepare bucket %q: %w", bucket, err)
		}
	}

	head, err := s.Storage.ObjectHead(ctx, sellerBucket, input.AssetKey)
	if err != nil {
		return HandoffResult{}, fmt.Errorf("inspect seller asset: %w", err)
	}
	if !head.Found {
		return HandoffResult{}, ErrAssetNotReady
	}

	record := handoffRecord{
		OrderID: input.OrderID, SellerTenant: input.SellerTenant, BuyerTenant: input.BuyerTenant,
		AssetBucket: sellerBucket, AssetKey: input.AssetKey, Status: "ready_for_buyer",
	}
	encoded, err := encodeRecord(record)
	if err != nil {
		return HandoffResult{}, err
	}
	buyerUpdateKey := "updates/" + input.OrderID + ".json"
	handoffKey := "handoffs/" + input.OrderID + ".json"
	requestID := "handoff-" + input.OrderID
	if err := s.Storage.ObjectPut(ctx, buyerBucket, buyerUpdateKey, encoded, requestID+"-buyer"); err != nil {
		return HandoffResult{}, fmt.Errorf("write buyer update: %w", err)
	}
	if err := s.Storage.ObjectPut(ctx, orderBucket, handoffKey, encoded, requestID+"-order"); err != nil {
		return HandoffResult{}, fmt.Errorf("write order handoff: %w", err)
	}

	return HandoffResult{
		OrderID: input.OrderID, Status: record.Status, SellerBucket: sellerBucket,
		BuyerBucket: buyerBucket, OrderBucket: orderBucket, BuyerUpdateKey: buyerUpdateKey, HandoffKey: handoffKey,
	}, nil
}

func tenantBucket(role, tenantID string) string {
	sum := sha256.Sum256([]byte(tenantID))
	suffix := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:5]))
	return "market-" + role + "-" + suffix
}

func encodeRecord(record handoffRecord) (string, error) {
	raw, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("encode handoff: %w", err)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}
