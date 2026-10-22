package brontosdk

import (
	"context"
	"maps"
	"net/http"
	"net/url"
	"slices"

	"github.com/teamwork/bronto-mcp-go/internal/jsonvalue"
)

// Datasets lists every dataset in the organisation. Each item is an object
// carrying at least log (the dataset name), logset (its collection), log_id and
// tags.
func (c *Client) Datasets(ctx context.Context) ([]any, error) {
	body, err := c.do(ctx, http.MethodGet, "logs", nil, nil)
	if err != nil {
		return nil, err
	}
	return jsonvalue.List(body, "logs"), nil
}

// topKeys fetches the top-keys catalogue, narrowed to one dataset when logID is
// set. The response maps each dataset id to its keys, and each key to an object
// whose "values" field maps sample values to their counts.
func (c *Client) topKeys(ctx context.Context, logID string) (map[string]any, error) {
	var query url.Values
	if logID != "" {
		query = url.Values{"log_id": {logID}}
	}
	body, err := c.do(ctx, http.MethodGet, "top-keys", query, nil)
	if err != nil {
		return nil, err
	}
	return object(body)
}

// DatasetKeys returns the names of the keys present in one dataset, sorted. A
// dataset with no top keys yet has none.
func (c *Client) DatasetKeys(ctx context.Context, logID string) ([]string, error) {
	body, err := c.topKeys(ctx, logID)
	if err != nil {
		return nil, err
	}
	return sortedKeys(jsonvalue.Map(body[logID])), nil
}

// AllDatasetKeys maps every dataset id to the sorted names of its keys.
func (c *Client) AllDatasetKeys(ctx context.Context) (map[string][]string, error) {
	body, err := c.topKeys(ctx, "")
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(body))
	for logID, keys := range body {
		result[logID] = sortedKeys(jsonvalue.Map(keys))
	}
	return result, nil
}

// DatasetKeyValues returns the sorted sample values seen for one key of one
// dataset. A key the dataset does not have has none.
func (c *Client) DatasetKeyValues(ctx context.Context, logID, key string) ([]string, error) {
	body, err := c.topKeys(ctx, logID)
	if err != nil {
		return nil, err
	}
	detail := jsonvalue.Map(jsonvalue.Map(body[logID])[key])
	return sortedKeys(jsonvalue.Map(detail["values"])), nil
}

// sortedKeys returns an object's keys in sorted order, never nil.
func sortedKeys(m map[string]any) []string {
	keys := slices.Sorted(maps.Keys(m))
	if keys == nil {
		return []string{}
	}
	return keys
}
