package server

import (
	"context"
	"time"

	"g9router/internal/providers"
)

func (s *Server) credentialProvider(ctx context.Context, provider providers.Provider, refresh bool) (providers.Provider, error) {
	if provider.OAuthID == "" {
		return provider, nil
	}
	credential, ok := s.oauth.Get(provider.OAuthID)
	if !ok {
		return provider, nil
	}
	if refresh && credential.ExpiringSoon(time.Now()) && credential.RefreshToken != "" {
		refreshed, err := s.oauth.Refresh(ctx, provider.OAuthID)
		if err != nil {
			return provider, err
		}
		credential = refreshed
	}
	provider.APIKey = credential.AccessToken
	return provider, nil
}
