package openlist

import (
	"context"

	"github.com/Soltus/encv-go/internal/config"
)

type MultiSiteServer struct {
	ctx          context.Context
	cfg          *config.Config
	tokenManager *TokenManager
}

func NewMultiSiteServer(ctx context.Context) *MultiSiteServer {
	cfg := config.FromContext(ctx)
	tokenManager := NewTokenManager(ctx)

	return &MultiSiteServer{
		ctx:          ctx,
		cfg:          cfg,
		tokenManager: tokenManager,
	}
}

func (m *MultiSiteServer) GetConfig() *config.Config {
	return m.cfg
}

func (m *MultiSiteServer) GetTokenManager() *TokenManager {
	return m.tokenManager
}
