package marketplace

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeStorage struct {
	found   bool
	buckets []string
	puts    []string
}

func (f *fakeStorage) EnsureBucket(_ context.Context, bucket string) error {
	f.buckets = append(f.buckets, bucket)
	return nil
}

func (f *fakeStorage) ObjectHead(context.Context, string, string) (HeadResult, error) {
	return HeadResult{Found: f.found}, nil
}

func (f *fakeStorage) ObjectPut(_ context.Context, bucket, key, _, idempotencyKey string) error {
	f.puts = append(f.puts, bucket+"/"+key+"#"+idempotencyKey)
	return nil
}

func TestHandoffDecision(t *testing.T) {
	tests := []struct {
		name      string
		found     bool
		wantState string
		wantErr   error
		wantPuts  int
	}{
		{name: "available seller asset produces buyer update and order record", found: true, wantState: "ready_for_buyer", wantPuts: 2},
		{name: "missing seller asset stops handoff", found: false, wantErr: ErrAssetNotReady, wantPuts: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &fakeStorage{found: tt.found}
			service := HandoffService{Storage: storage}
			got, err := service.Handoff(context.Background(), HandoffRequest{
				SellerTenant: "seller_204", BuyerTenant: "buyer_731", OrderID: "ord-8842", AssetKey: "assets/receipt.pdf",
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Handoff() error = %v, want %v", err, tt.wantErr)
			}
			if got.Status != tt.wantState {
				t.Fatalf("Handoff() status = %q, want %q", got.Status, tt.wantState)
			}
			if len(storage.puts) != tt.wantPuts {
				t.Fatalf("ObjectPut calls = %d, want %d", len(storage.puts), tt.wantPuts)
			}
			wantBuckets := []string{tenantBucket("seller", "seller_204"), tenantBucket("buyer", "buyer_731"), tenantBucket("orders", "seller_204")}
			if !reflect.DeepEqual(storage.buckets, wantBuckets) {
				t.Fatalf("buckets = %v, want %v", storage.buckets, wantBuckets)
			}
		})
	}
}
