package service

import (
	"context"

	"api-sync-go/internal/model"
)

// UserProvider defines the interface for querying user information by SteamID64.
// Both standard UserService and legacy.UserService implement this interface.
type UserProvider interface {
	GetBySteamID(ctx context.Context, steamID64 string) (*model.UserInfo, error)
}
