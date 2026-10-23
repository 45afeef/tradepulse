package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"tradepulse/internal/domain/user"
)

// authMiddleware validates the Bearer JWT, then loads the user on EVERY request so
// suspension and role changes take effect immediately. Suspended or unknown users
// get the same 401 as an invalid token. adminOnly additionally requires role admin.
func (s *Server) authMiddleware(api huma.API, adminOnly bool) func(huma.Context, func(huma.Context)) {
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
		u, err := s.Users.GetByID(ctx.Context(), claims.Sub)
		if err != nil || u.Status != user.StatusActive {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		issued := time.Unix(claims.Iat, 0)
		if claims.Ian != 0 {
			issued = time.Unix(0, claims.Ian)
		}
		if issued.Before(u.SessionsValidAfter) { // logged out everywhere / password changed or reset
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		if adminOnly && u.Role != user.RoleAdmin {
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "administrator access required")
			return
		}
		next(huma.WithValue(huma.WithValue(ctx, userKey, u.ID), roleKey, u.Role))
	}
}
