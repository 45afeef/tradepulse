// Package audit records who did what (admin and security-sensitive actions).
package audit

import (
	"context"
	"time"
)

type Entry struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	ActorID    string    `json:"actor_id" doc:"User who performed the action"`
	Action     string    `json:"action" example:"invite.create"`
	TargetType string    `json:"target_type,omitempty"`
	TargetID   string    `json:"target_id,omitempty"`
	Detail     string    `json:"detail,omitempty"`
}

type Repository interface {
	Append(ctx context.Context, e Entry) error
	// Recent returns the newest entries first.
	Recent(ctx context.Context, limit int) ([]Entry, error)
}
