package service

import (
	"context"

	"api-sync-go/internal/model"
)

type UserProvider interface {
	GetBySteamID(ctx context.Context, steamID64 string) (*model.UserInfo, error)
}
