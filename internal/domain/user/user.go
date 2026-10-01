// Package user holds user identity and the personalization profile.
package user

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"tradepulse/internal/domain/event"
)

var (
	ErrNotFound   = errors.New("user not found")
	ErrEmailTaken = errors.New("email already registered")
)

type Profile struct {
	Watchlist             []string `json:"watchlist"`
	EventTypes            []string `json:"event_types"` // empty = all types
	Muted                 []string `json:"muted"`
	VolumeSpikeThreshold  float64  `json:"volume_spike_threshold"`
	PriceMoveThresholdPct float64  `json:"price_move_threshold_pct"`
}

const (
	defaultVolumeThreshold = 2.0
	defaultPriceThreshold  = 1.0
	maxSymbols             = 200
)

func DefaultProfile() Profile {
	return Profile{
		Watchlist: []string{}, EventTypes: []string{}, Muted: []string{},
		VolumeSpikeThreshold:  defaultVolumeThreshold,
		PriceMoveThresholdPct: defaultPriceThreshold,
	}
}

var symbolRe = regexp.MustCompile(`^[A-Z0-9&-]{1,20}$`)

func normSymbols(in []string, field string) ([]string, error) {
	if len(in) > maxSymbols {
		return nil, fmt.Errorf("%s: at most %d entries", field, maxSymbols)
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToUpper(strings.TrimSpace(s))
		if !symbolRe.MatchString(s) {
			return nil, fmt.Errorf("%s: invalid symbol %q", field, s)
		}
		if _, dup := seen[s]; !dup {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out, nil
}

// Normalize validates and canonicalizes a profile.
func (p Profile) Normalize() (Profile, error) {
	var err error
	out := Profile{}
	if out.Watchlist, err = normSymbols(p.Watchlist, "watchlist"); err != nil {
		return Profile{}, err
	}
	if out.Muted, err = normSymbols(p.Muted, "muted"); err != nil {
		return Profile{}, err
	}
	seen := map[string]struct{}{}
	out.EventTypes = make([]string, 0, len(p.EventTypes))
	for _, t := range p.EventTypes {
		t = strings.ToLower(strings.TrimSpace(t))
		if !event.IsKnown(t) {
			return Profile{}, fmt.Errorf("event_types: unknown type %q", t)
		}
		if _, dup := seen[t]; !dup {
			seen[t] = struct{}{}
			out.EventTypes = append(out.EventTypes, t)
		}
	}
	out.VolumeSpikeThreshold = p.VolumeSpikeThreshold
	if out.VolumeSpikeThreshold == 0 {
		out.VolumeSpikeThreshold = defaultVolumeThreshold
	}
	if out.VolumeSpikeThreshold < 1 || out.VolumeSpikeThreshold > 100 {
		return Profile{}, errors.New("volume_spike_threshold must be between 1 and 100")
	}
	out.PriceMoveThresholdPct = p.PriceMoveThresholdPct
	if out.PriceMoveThresholdPct == 0 {
		out.PriceMoveThresholdPct = defaultPriceThreshold
	}
	if out.PriceMoveThresholdPct < 0.05 || out.PriceMoveThresholdPct > 50 {
		return Profile{}, errors.New("price_move_threshold_pct must be between 0.05 and 50")
	}
	return out, nil
}

type User struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	Profile      Profile
}

type Repository interface {
	Create(ctx context.Context, u User) error
	GetByID(ctx context.Context, id string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
	UpdateProfile(ctx context.Context, id string, p Profile) error
	List(ctx context.Context) ([]User, error)
}
