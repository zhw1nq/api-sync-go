package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"api-sync-go/internal/model"
)

func APIKeyAuth(validKeys []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-KEY")
			if apiKey == "" {
				writeError(w, http.StatusUnauthorized, "missing X-API-KEY header", "UNAUTHORIZED")
				return
			}

			if !validateKey(apiKey, validKeys) {
				writeError(w, http.StatusUnauthorized, "invalid API key", "UNAUTHORIZED")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func validateKey(provided string, validKeys []string) bool {
	providedBytes := []byte(provided)
	for _, key := range validKeys {
		if subtle.ConstantTimeCompare(providedBytes, []byte(key)) == 1 {
			return true
		}
	}
	return false
}

func writeError(w http.ResponseWriter, status int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(model.APIResponse{
		Success: false,
		Error:   message,
		Code:    code,
	})
}
