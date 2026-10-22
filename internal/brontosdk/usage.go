package brontosdk

import (
	"context"
	"net/http"
	"net/url"

	"github.com/teamwork/bronto-mcp-go/internal/jsonvalue"
)

// Usage types accepted by UsageByLog.
const (
	UsageIngestion = "ingestion"
	UsageSearch    = "search"
)

// usageLimit caps how many datasets one usage request reports on.
const usageLimit = "500"

// UsageRow is one dataset's usage over a window.
type UsageRow struct {
	LogID      string
	BytesTotal int64
	Count      int64
}

// UsageByLog reports per-dataset usage of one type over a relative window such
// as "This month". Only billable usage is reported: system collections such as
// .usage and .audit-trail are left out.
func (c *Client) UsageByLog(ctx context.Context, usageType, timeRange string) ([]UsageRow, error) {
	query := url.Values{
		"usage_type": {usageType},
		"delta":      {"true"},
		"limit":      {usageLimit},
		"time_range": {timeRange},
	}
	body, err := c.do(ctx, http.MethodGet, "usage/organizations/logs", query, nil)
	if err != nil {
		return nil, err
	}

	var rows []UsageRow
	for _, item := range jsonvalue.List(body, "groups_series") {
		group := jsonvalue.Map(item)
		if group == nil || group["name"] == nil {
			continue
		}
		rows = append(rows, UsageRow{
			LogID:      jsonvalue.String(group["name"]),
			BytesTotal: jsonvalue.Int(group["value"]),
			Count:      jsonvalue.Int(group["count"]),
		})
	}
	return rows, nil
}

// Limits lists the organisation's usage limits.
func (c *Client) Limits(ctx context.Context) ([]any, error) {
	body, err := c.do(ctx, http.MethodGet, "limits", nil, nil)
	if err != nil {
		return nil, err
	}
	return jsonvalue.List(body, "limits"), nil
}

// ErrorAnalytics returns the precomputed parsing and ingestion error analytics
// over a relative window such as "Last 24 hours".
func (c *Client) ErrorAnalytics(ctx context.Context, timeRange string) (any, error) {
	return c.do(ctx, http.MethodGet, "analytics/errors", url.Values{"time_range": {timeRange}}, nil)
}
