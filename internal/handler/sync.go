package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"api-sync-go/internal/model"
	"api-sync-go/internal/service"
)

func isValidSteamID(s string) bool {
	if len(s) != 17 {
		return false
	}
	for i := 0; i < 17; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

type SyncHandler struct {
	userService service.UserProvider
	logger      *slog.Logger
}

func NewSyncHandler(us service.UserProvider, logger *slog.Logger) *SyncHandler {
	return &SyncHandler{userService: us, logger: logger}
}

func (h *SyncHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	steamID := r.PathValue("steamid")

	if !isValidSteamID(steamID) {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{
			Success: false,
			Error:   "invalid steamid format, must be 17 digits",
			Code:    "INVALID_STEAMID",
		})
		return
	}

	avatarURL := buildAvatarURL(r, steamID)

	info, err := h.userService.GetBySteamID(r.Context(), steamID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			shortSteamID := strings.TrimPrefix(steamID, "7656119")
			writeJSON(w, http.StatusOK, model.APIResponse{
				Success: false,
				Data: model.UserInfo{
					Name:  fmt.Sprintf("[GUEST] %s", shortSteamID),
					Image: avatarURL,
				},
			})
			return
		}
		h.logger.Error("get user failed", slog.String("steamid", steamID), slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Error:   "internal server error",
			Code:    "INTERNAL_ERROR",
		})
		return
	}

	writeJSON(w, http.StatusOK, model.APIResponse{
		Success: true,
		Data: model.UserInfo{
			Name:  info.Name,
			Image: avatarURL,
		},
	})
}

func buildAvatarURL(r *http.Request, steamID string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if xfHost := r.Header.Get("X-Forwarded-Host"); xfHost != "" {
		host = xfHost
	}
	if host != "" {
		return fmt.Sprintf("%s://%s/api/sync/%s/avatar.jpg", scheme, host, steamID)
	}
	return fmt.Sprintf("/api/sync/%s/avatar.jpg", steamID)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
