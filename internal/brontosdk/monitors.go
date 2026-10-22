package brontosdk

import (
	"context"
	"net/http"
	"net/url"

	"github.com/teamwork/bronto-mcp-go/internal/jsonvalue"
)

// Users lists the organisation's users. Each carries at least an id and an
// email, which is how a monitor's notification target is checked.
func (c *Client) Users(ctx context.Context) ([]any, error) {
	body, err := c.do(ctx, http.MethodGet, "users", nil, nil)
	if err != nil {
		return nil, err
	}
	return jsonvalue.List(body, "users"), nil
}

// CreateMonitor creates a monitor and returns it as Bronto answered.
func (c *Client) CreateMonitor(ctx context.Context, body any) (any, error) {
	return c.do(ctx, http.MethodPost, "monitors", nil, body)
}

// Monitors lists every monitor.
func (c *Client) Monitors(ctx context.Context) ([]any, error) {
	body, err := c.do(ctx, http.MethodGet, "monitors", nil, nil)
	if err != nil {
		return nil, err
	}
	return jsonvalue.List(body, "monitors"), nil
}

// MonitorEvents lists a monitor's recent trigger and recovery events.
func (c *Client) MonitorEvents(ctx context.Context, monitorID string) ([]any, error) {
	body, err := c.do(ctx, http.MethodGet, "monitors/"+url.PathEscape(monitorID)+"/events", nil, nil)
	if err != nil {
		return nil, err
	}
	return jsonvalue.List(body, "events"), nil
}

// MetricDefinitions lists the metric definitions that back monitors. A
// monitor's metric_id names one, and its queries say what the monitor reads.
func (c *Client) MetricDefinitions(ctx context.Context) ([]any, error) {
	body, err := c.do(ctx, http.MethodGet, "metrics/definitions", nil, nil)
	if err != nil {
		return nil, err
	}
	return jsonvalue.List(body, "definitions"), nil
}
