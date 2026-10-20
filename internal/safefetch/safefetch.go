// Package safefetch is the ONLY way the server may make outbound HTTP requests to
// URLs that an admin, a source config or a user supplied (SSRF protection).
//
// Guarantees: http/https only, ports 80/443 only, no credentials in URLs, cloud
// metadata hosts blocked, every connection's IP is validated AT DIAL TIME (so DNS
// rebinding and redirects to internal hosts cannot bypass it) and the validated IP
// is the one dialed, bounded redirects/time/size, no proxy, no cookies.
package safefetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var (
	ErrBlocked          = errors.New("safefetch: destination not allowed")
	ErrScheme           = errors.New("safefetch: only http and https URLs are allowed")
	ErrPort             = errors.New("safefetch: only ports 80 and 443 are allowed")
	ErrURL              = errors.New("safefetch: invalid URL")
	ErrTooLarge         = errors.New("safefetch: response too large")
	ErrTooManyRedirects = errors.New("safefetch: too many redirects")
)

type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type Options struct {
	Timeout      time.Duration // whole request, default 10s
	MaxBytes     int64         // response body cap, default 2 MiB
	MaxRedirects int           // default 3
	UserAgent    string        // default "TradePulse/1.0 (+source-reader)"
	Resolver     Resolver      // default net.DefaultResolver
}

type Response struct {
	Status   int
	Header   http.Header
	Body     []byte
	FinalURL string
}

type Fetcher struct {
	client   *http.Client
	post     *http.Client // never follows redirects (webhook deliveries)
	maxBytes int64
	ua       string
	resolver Resolver

	// test hooks (unexported on purpose: production code cannot relax the policy)
	allow        func(netip.Addr) bool
	allowAnyPort bool
}

func New(o Options) *Fetcher {
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Second
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = 2 << 20
	}
	if o.MaxRedirects <= 0 {
		o.MaxRedirects = 3
	}
	if o.UserAgent == "" {
		o.UserAgent = "TradePulse/1.0 (+source-reader)"
	}
	if o.Resolver == nil {
		o.Resolver = net.DefaultResolver
	}
	f := &Fetcher{maxBytes: o.MaxBytes, ua: o.UserAgent, resolver: o.Resolver}
	f.build(o.Timeout, o.MaxRedirects)
	return f
}

// build (re)creates the client; tests call it again after setting hooks.
func (f *Fetcher) build(timeout time.Duration, maxRedirects int) {
	tr := &http.Transport{
		Proxy:                 nil, // never honor HTTP(S)_PROXY from the environment
		DialContext:           f.dial,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          20,
		IdleConnTimeout:       30 * time.Second,
		DisableKeepAlives:     false,
	}
	f.post = &http.Client{Transport: tr, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	f.client = &http.Client{
		Transport: tr, // no cookie jar
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return ErrTooManyRedirects
			}
			return f.validateURL(req.URL)
		},
	}
}

var blockedHosts = map[string]bool{
	"metadata": true, "metadata.google.internal": true, "localhost": true,
	"instance-data": true, "kubernetes.default.svc": true,
}

func (f *Fetcher) validateURL(u *url.URL) error {
	if u == nil {
		return ErrURL
	}
	// Report a disallowed scheme (file:, ftp:, javascript:...) before looking at the host.
	if u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https" {
		return ErrScheme
	}
	if u.Scheme == "" || u.Host == "" || u.Hostname() == "" {
		return ErrURL
	}
	if u.User != nil {
		return ErrURL // no credentials in URLs
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if blockedHosts[host] || strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".localhost") {
		return ErrBlocked
	}
	if p := u.Port(); p != "" && p != "80" && p != "443" && !f.allowAnyPort {
		return ErrPort
	}
	return nil
}

var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "64:ff9b::/96", "2001:db8::/32",
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// isBlocked reports whether an address must never be contacted.
func isBlocked(a netip.Addr) bool {
	a = a.Unmap() // ::ffff:127.0.0.1 is 127.0.0.1
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsInterfaceLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func (f *Fetcher) allowed(a netip.Addr) bool {
	if f.allow != nil {
		return f.allow(a.Unmap())
	}
	return !isBlocked(a)
}

// pickAllowed keeps only addresses we may dial.
func (f *Fetcher) pickAllowed(addrs []netip.Addr) []netip.Addr {
	var out []netip.Addr
	for _, a := range addrs {
		if f.allowed(a) {
			out = append(out, a.Unmap())
		}
	}
	return out
}

// dial resolves, validates and connects to the validated IP (no second lookup).
func (f *Fetcher) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrURL
	}
	if port != "80" && port != "443" && !f.allowAnyPort {
		return nil, ErrPort
	}
	addrs, err := f.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("safefetch: resolve: %w", err)
	}
	ok := f.pickAllowed(addrs)
	if len(ok) == 0 {
		return nil, ErrBlocked
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	var lastErr error
	for _, a := range ok {
		c, err := d.DialContext(ctx, network, net.JoinHostPort(a.String(), port))
		if err == nil {
			return c, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// Get fetches rawURL. extra headers (e.g. If-None-Match) are copied; Authorization
// and Cookie are never forwarded by this package unless the caller passes them.
func (f *Fetcher) Get(ctx context.Context, rawURL string, extra http.Header) (*Response, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, ErrURL
	}
	if err := f.validateURL(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ErrURL
	}
	for k, vs := range extra {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("User-Agent", f.ua)
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > f.maxBytes {
		return nil, ErrTooLarge
	}
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: body, FinalURL: resp.Request.URL.String()}, nil
}

// ValidateURL performs the static checks (scheme, port, credentials, blocked host
// names and IP literals) without any network access. Use it to reject bad URLs at
// configuration time; Get re-validates at fetch time, including DNS answers.
func ValidateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ErrURL
	}
	f := &Fetcher{}
	if err := f.validateURL(u); err != nil {
		return err
	}
	if a, err := netip.ParseAddr(u.Hostname()); err == nil && isBlocked(a) {
		return ErrBlocked
	}
	return nil
}

// Post sends a body (e.g. a signed webhook delivery) with the same protections as Get
// (IP validation at dial time, ports, timeouts, no proxy, no cookies). Redirects are
// NOT followed: a 3xx is returned as-is so callers can treat it as a failure. Only the
// first 4 KB of the response body is returned.
func (f *Fetcher) Post(ctx context.Context, rawURL string, body []byte, hdr http.Header) (*Response, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, ErrURL
	}
	if err := f.validateURL(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, ErrURL
	}
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", f.ua)
	resp, err := f.post.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: b, FinalURL: u.String()}, nil
}
