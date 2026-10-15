package api

import (
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// authMiddleware validates the Bearer JWT and stores the user ID in the context.
func (s *Server) authMiddleware(api huma.API) func(huma.Context, func(huma.Context)) {
	const prefix = "Bearer "
	return func(ctx huma.Context, next func(huma.Context)) {
		h := ctx.Header("Authorization")
		if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := s.JWT.Verify(h[len(prefix):])
		if err != nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		next(huma.WithValue(ctx, userKey, claims.Sub))
	}
}
