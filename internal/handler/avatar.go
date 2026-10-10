package handler

import (
	"errors"
	"fmt"
	"hash/crc32"
	"log/slog"
	"net/http"
	"strconv"

	"api-sync-go/internal/model"
	"api-sync-go/internal/service"
)

type AvatarHandler struct {
	userService  service.UserProvider
	imageService *service.ImageService
	logger       *slog.Logger
}

func NewAvatarHandler(us service.UserProvider, is *service.ImageService, logger *slog.Logger) *AvatarHandler {
	return &AvatarHandler{userService: us, imageService: is, logger: logger}
}

func (h *AvatarHandler) GetAvatar(w http.ResponseWriter, r *http.Request) {
	steamID := r.PathValue("steamid")

	if !isValidSteamID(steamID) {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{
			Success: false,
			Error:   "invalid steamid format, must be 17 digits",
			Code:    "INVALID_STEAMID",
		})
		return
	}

	cached, err := h.imageService.GetCachedAvatar(r.Context(), steamID)
	if err == nil && len(cached) > 0 {
		h.serveJPEG(w, r, steamID, cached)
		return
	}

	info, err := h.userService.GetBySteamID(r.Context(), steamID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			defaultAvatar := h.imageService.GetDefaultAvatar()
			_ = h.imageService.CacheDefaultAvatar(r.Context(), steamID)
			h.serveJPEG(w, r, steamID, defaultAvatar)
			return
		}
		h.logger.Error("get user for avatar failed", slog.String("steamid", steamID), slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Error:   "internal server error",
			Code:    "INTERNAL_ERROR",
		})
		return
	}

	if info.Image == "" {
		defaultAvatar := h.imageService.GetDefaultAvatar()
		_ = h.imageService.CacheDefaultAvatar(r.Context(), steamID)
		h.serveJPEG(w, r, steamID, defaultAvatar)
		return
	}

	jpegBytes, err := h.imageService.GetCompressedAvatar(r.Context(), steamID, info.Image)
	if err != nil {
		h.logger.Warn("compress avatar failed, falling back to default",
			slog.String("steamid", steamID),
			slog.String("imageURL", info.Image),
			slog.String("error", err.Error()),
		)
		defaultAvatar := h.imageService.GetDefaultAvatar()
		_ = h.imageService.CacheDefaultAvatar(r.Context(), steamID)
		h.serveJPEG(w, r, steamID, defaultAvatar)
		return
	}

	h.serveJPEG(w, r, steamID, jpegBytes)
}

func (h *AvatarHandler) serveJPEG(w http.ResponseWriter, r *http.Request, steamID string, jpegBytes []byte) {
	if len(jpegBytes) == 0 {
		h.logger.Warn("serving empty avatar bytes", slog.String("steamid", steamID))
	}

	etag := fmt.Sprintf(`W/"%x-%x"`, len(jpegBytes), crc32.ChecksumIEEE(jpegBytes))
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(jpegBytes)))
	w.Header().Set("Cache-Control", "public, max-age=1800, stale-while-revalidate=86400")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.jpg"`, steamID))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(jpegBytes)
}
