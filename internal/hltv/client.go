package hltv

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"psr/internal/config"
)

type Client struct {
	cfg     config.Config
	fetcher Fetcher
	session *Session
}

func NewClient(cfg config.Config) (*Client, error) {
	if cfg.Cookie == "" {
		cfg.Cookie = LoadStoredCookie(cfg)
	}
	sess := NewSession(cfg)
	return &Client{cfg: cfg, fetcher: sess, session: sess}, nil
}

func (c *Client) EnsureSession(ctx context.Context) error {
	if c.session != nil {
		return c.session.Ensure(ctx)
	}
	return nil
}

func (c *Client) SetCookie(ctx context.Context, cookie string) error {
	if c.session == nil {
		return ErrCookieRequired
	}
	if err := c.session.SetCookie(ctx, cookie); err != nil {
		return err
	}
	c.cfg.Cookie = cookie
	return nil
}

func (c *Client) Connect(ctx context.Context) error {
	if c.session != nil {
		return c.session.Connect(ctx)
	}
	return ErrCookieRequired
}

func (c *Client) CookieStatus(ctx context.Context) (has bool, valid bool) {
	if c.session == nil {
		return false, false
	}
	return c.session.CookieStatus(ctx)
}

func (c *Client) Fetch(ctx context.Context, path string) (string, error) {
	target, err := resolveHLTVURL(c.cfg.BaseURL, path)
	if err != nil {
		return "", err
	}
	body, err := c.fetcher.Fetch(ctx, target)
	if err != nil {
		return "", err
	}
	return body, nil
}

// resolveHLTVURL joins base URL with a path that may include a query string.
func resolveHLTVURL(baseURL, path string) (string, error) {
	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	rel, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("parse path: %w", err)
	}
	return base.ResolveReference(rel).String(), nil
}

func (c *Client) Close() error {
	if c.fetcher != nil {
		return c.fetcher.Close()
	}
	return nil
}
