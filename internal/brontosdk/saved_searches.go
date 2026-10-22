package brontosdk

import (
	"context"
	"net/http"
	"net/url"
)

// SavedSearches lists every saved search.
func (c *Client) SavedSearches(ctx context.Context) (any, error) {
	return c.do(ctx, http.MethodGet, "saved-searches", nil, nil)
}

// SavedSearch fetches one saved search by id.
func (c *Client) SavedSearch(ctx context.Context, id string) (any, error) {
	return c.do(ctx, http.MethodGet, "saved-searches/"+url.PathEscape(id), nil, nil)
}

// CreateSavedSearch creates a saved search and returns it as Bronto answered.
func (c *Client) CreateSavedSearch(ctx context.Context, body any) (any, error) {
	return c.do(ctx, http.MethodPost, "saved-searches", nil, body)
}
