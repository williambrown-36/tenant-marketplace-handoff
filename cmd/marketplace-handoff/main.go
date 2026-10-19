package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	marketplace "github.com/example/tenant-marketplace-handoff"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	service := marketplace.HandoffService{Storage: marketplace.NewClient(apiKey)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /seller-assets", sellerAssetHandler(service))
	mux.HandleFunc("POST /handoffs", handoffHandler(service))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("marketplace handoff service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func sellerAssetHandler(service marketplace.HandoffService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var input marketplace.SellerAssetRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		result, err := service.StoreSellerAsset(r.Context(), input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

func handoffHandler(service marketplace.HandoffService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var input marketplace.HandoffRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		result, err := service.Handoff(r.Context(), input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

func writeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, marketplace.ErrAssetNotReady) {
		status = http.StatusConflict
	}
	var apiErr *marketplace.APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		status = apiErr.HTTPStatus
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
