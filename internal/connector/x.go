package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"tradepulse/internal/domain/sourcedef"
)

// XSettings come from environment variables (see .env.example); nothing is hardcoded.
type XSettings struct {
	Token              string  // X_BEARER_TOKEN (secret)
	BaseURL            string  // X_API_BASE_URL
	BudgetUSD          float64 // X_MONTHLY_BUDGET_USD: hard monthly cap
	CostPerResourceUSD float64 // X_COST_PER_RESOURCE_USD
	MaxPosts           int     // X_MAX_POSTS_PER_FETCH (5-100)
}

var ErrXBudget = errors.New("monthly X budget reached")

const usageKeyX = "x"

// X reads an account's recent posts through the official API only. Cost control:
// polling uses since_id (billed per post returned, so only NEW posts cost money),
// and every call is checked against a persistent monthly budget BEFORE it is made.
type X struct {
	http  HTTP
	cfg   XSettings
	usage sourcedef.UsageStore
	now   func() time.Time
}

func NewX(h HTTP, cfg XSettings, usage sourcedef.UsageStore) *X {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.x.com"
	}
	if cfg.MaxPosts < 5 || cfg.MaxPosts > 100 {
		cfg.MaxPosts = 20
	}
	if cfg.CostPerResourceUSD <= 0 {
		cfg.CostPerResourceUSD = 0.005
	}
	return &X{http: h, cfg: cfg, usage: usage, now: time.Now}
}

func (*X) Type() sourcedef.Type { return sourcedef.TypeX }

func (x *X) Validate(src sourcedef.Source) error {
	if x.cfg.Token == "" {
		return errors.New("X is not configured on this server (set X_BEARER_TOKEN)")
	}
	if src.X == nil || src.X.Handle == "" {
		return errors.New("x.handle is required")
	}
	return nil
}

func (x *X) costMicro() int64 {
	c := int64(x.cfg.CostPerResourceUSD*1e6 + 0.5)
	if c < 1 {
		c = 1
	}
	return c
}

func (x *X) budgetMicro() int64 { return int64(x.cfg.BudgetUSD*1e6 + 0.5) }

// affordable returns how many resources can still be bought this month.
func (x *X) affordable(ctx context.Context) (int, error) {
	spent, err := x.usage.Month(ctx, usageKeyX, x.now())
	if err != nil {
		return 0, fmt.Errorf("usage ledger unavailable: %w", err)
	}
	left := x.budgetMicro() - spent
	if left <= 0 {
		return 0, nil
	}
	n := left / x.costMicro()
	if n > 1<<20 {
		n = 1 << 20
	}
	return int(n), nil
}

func (x *X) get(ctx context.Context, path string, q url.Values) (*xResp, error) {
	u := strings.TrimRight(x.cfg.BaseURL, "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+x.cfg.Token)
	hdr.Set("Accept", "application/json")
	resp, err := x.http.Get(ctx, u, hdr)
	if err != nil {
		return nil, redact(err, x.cfg.Token)
	}
	switch {
	case resp.Status == 401 || resp.Status == 403:
		return nil, fmt.Errorf("X API rejected the request (HTTP %d): check X_BEARER_TOKEN and your API access tier", resp.Status)
	case resp.Status == 402:
		return nil, errors.New("X API credits are exhausted (HTTP 402)")
	case resp.Status == 429:
		return nil, errors.New("X API rate limit reached (HTTP 429)")
	case resp.Status < 200 || resp.Status >= 300:
		return nil, fmt.Errorf("unexpected X API status %d", resp.Status)
	}
	return &xResp{body: resp.Body, status: resp.Status}, nil
}

type xResp struct {
	body   []byte
	status int
}

func redact(err error, secret string) error {
	if secret != "" && strings.Contains(err.Error(), secret) {
		return errors.New("request failed")
	}
	return err
}

type xUserDoc struct {
	Data struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"data"`
}

// Resolve looks up the numeric user id for the handle (one billed read) and stores it
// on the source, so polling never has to look it up again.
func (x *X) Resolve(ctx context.Context, src sourcedef.Source) (sourcedef.Source, error) {
	if err := x.Validate(src); err != nil {
		return src, err
	}
	n, err := x.affordable(ctx)
	if err != nil {
		return src, err
	}
	if n < 1 {
		return src, ErrXBudget
	}
	resp, err := x.get(ctx, "/2/users/by/username/"+url.PathEscape(src.X.Handle), nil)
	if err != nil {
		return src, err
	}
	var doc xUserDoc
	if err := json.Unmarshal(resp.body, &doc); err != nil || doc.Data.ID == "" {
		return src, errors.New("X account not found")
	}
	if _, err := x.usage.Add(ctx, usageKeyX, x.costMicro(), x.now()); err != nil {
		return src, fmt.Errorf("usage ledger unavailable: %w", err)
	}
	cp := *src.X
	cp.UserID = doc.Data.ID
	src.X = &cp
	return src, nil
}

type xTweetsDoc struct {
	Data []struct {
		ID        string `json:"id"`
		Text      string `json:"text"`
		CreatedAt string `json:"created_at"`
	} `json:"data"`
	Meta struct {
		NewestID    string `json:"newest_id"`
		ResultCount int    `json:"result_count"`
	} `json:"meta"`
}

func (x *X) Fetch(ctx context.Context, src sourcedef.Source) (Result, error) {
	if err := x.Validate(src); err != nil {
		return Result{}, err
	}
	if src.X.UserID == "" {
		return Result{}, errors.New("x source has no resolved user id: save the source again")
	}
	afford, err := x.affordable(ctx)
	if err != nil {
		return Result{}, err
	}
	max := x.cfg.MaxPosts
	if afford < max {
		max = afford
	}
	if max < 5 { // the API's minimum page size is 5
		spent, _ := x.usage.Month(ctx, usageKeyX, x.now())
		return Result{}, fmt.Errorf("%w ($%.2f of $%.2f used)", ErrXBudget, float64(spent)/1e6, x.cfg.BudgetUSD)
	}
	q := url.Values{}
	q.Set("max_results", strconv.Itoa(max))
	q.Set("tweet.fields", "created_at")
	q.Set("exclude", "retweets,replies")
	if src.State.Cursor != "" {
		q.Set("since_id", src.State.Cursor)
	}
	resp, err := x.get(ctx, "/2/users/"+url.PathEscape(src.X.UserID)+"/tweets", q)
	if err != nil {
		return Result{}, err
	}
	var doc xTweetsDoc
	if err := json.Unmarshal(resp.body, &doc); err != nil {
		return Result{HTTPStatus: resp.status}, errors.New("unexpected X API response")
	}
	// Charge before returning items so spend is never under-counted.
	if n := len(doc.Data); n > 0 {
		if _, err := x.usage.Add(ctx, usageKeyX, int64(n)*x.costMicro(), x.now()); err != nil {
			return Result{HTTPStatus: resp.status}, fmt.Errorf("usage ledger unavailable: %w", err)
		}
	}
	handle := src.X.Handle
	items := make([]Item, 0, len(doc.Data))
	for _, t := range doc.Data {
		text := collapse(html.UnescapeString(t.Text)) // API text is HTML-escaped; output stays plain text
		text = strings.Map(func(r rune) rune {
			if r < 0x20 && r != ' ' {
				return -1
			}
			return r
		}, text)
		if t.ID == "" || text == "" {
			continue
		}
		link := "https://x.com/" + url.PathEscape(handle) + "/status/" + url.PathEscape(t.ID)
		title := truncate(text, 280)
		raw, _ := json.Marshal(map[string]any{"title": title, "url": link, "id": t.ID, "created_at": t.CreatedAt})
		items = append(items, Item{
			Key: "x:" + t.ID, Title: title, URL: link, PublishedAt: parseDate(t.CreatedAt),
			Entities: []string{strings.ToLower(handle)}, Raw: raw,
		})
	}
	return Result{Items: items, HTTPStatus: resp.status, Cursor: doc.Meta.NewestID}, nil
}
