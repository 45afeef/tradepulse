// Package audit records who did what (admin and security-sensitive actions).
package audit

import (
	"context"
	"time"

	"tradepulse/internal/page"
)

type Entry struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	ActorID    string    `json:"actor_id" doc:"User who performed the action"`
	Action     string    `json:"action" example:"invite.create"`
	TargetType string    `json:"target_type,omitempty"`
	TargetID   string    `json:"target_id,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	Seq        int64     `json:"-"`
	Cursor     string    `json:"cursor" doc:"Opaque position; use as before/after"`
}

type Repository interface {
	Append(ctx context.Context, e Entry) error
	// Recent returns the newest entries first.
	Recent(ctx context.Context, limit int) ([]Entry, error)
	Page(ctx context.Context, q page.Query) ([]Entry, error)
}
