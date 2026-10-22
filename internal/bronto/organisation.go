package bronto

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teamwork/bronto-mcp-go/internal/brontosdk"
	"github.com/teamwork/bronto-mcp-go/internal/jsonvalue"
	"github.com/teamwork/mcp/pkg/helpers"
	"github.com/teamwork/mcp/pkg/toolsets"
)

// activeWindow is how recent a dataset's last heartbeat must be for it to
// count as still ingesting.
const activeWindow = 14 * 24 * time.Hour

func timeRangeSchema(description string) *jsonschema.Schema {
	return optional(&jsonschema.Schema{Type: "string", Description: description})
}

// GetErrorSummary summarises parsing and ingestion errors.
func GetErrorSummary(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name: string(MethodGetErrorSummary),
			Annotations: &mcp.ToolAnnotations{
				Title:           "Get Error Summary",
				ReadOnlyHint:    true,
				DestructiveHint: new(false),
				OpenWorldHint:   new(false),
			},
			Description: "Returns org-wide parsing/ingestion error and warning totals with a per-dataset " +
				"breakdown. Precomputed, so it costs no search quota. Optionally scoped by time range.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"time_range": timeRangeSchema("Relative window, e.g. \"Last 24 hours\", the default."),
				},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			arguments, err := helpers.NewToolArguments(request)
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}
			timeRange := arguments.GetString("time_range", "")
			if timeRange == "" {
				timeRange = "Last 24 hours"
			}

			raw, err := client.ErrorAnalytics(ctx, timeRange)
			if err != nil {
				return errorResult(err, "")
			}
			datasets, err := client.Datasets(ctx)
			if err != nil {
				return errorResult(err, "")
			}
			index := make(map[string]map[string]any, len(datasets))
			for _, item := range datasets {
				d := jsonvalue.Map(item)
				if jsonvalue.Truthy(d["log_id"]) {
					index[jsonvalue.String(d["log_id"])] = d
				}
			}
			return helpers.NewToolResultJSON(shapeErrorSummary(raw, index))
		},
	}
}

type datasetErrors struct {
	LogID      string  `json:"log_id"`
	Name       any     `json:"name"`
	Collection any     `json:"collection"`
	Errors     float64 `json:"errors"`
	Warnings   float64 `json:"warnings"`
}

type errorSummary struct {
	Errors   float64 `json:"errors"`
	Warnings float64 `json:"warnings"`
	// ByDataset lists only the datasets with errors or warnings, worst first.
	ByDataset []*datasetErrors `json:"by_dataset,omitempty"`
}

// shapeErrorSummary shapes an /analytics/errors payload.
//
// Its groups_series carries an "Error" and a "Warn" group, each with a total
// and a nested groups_series whose names are dataset ids. The datasets are
// named from index.
func shapeErrorSummary(raw any, index map[string]map[string]any) errorSummary {
	count := func(node map[string]any) float64 {
		if node["count"] != nil {
			return jsonvalue.Float(node["count"])
		}
		return jsonvalue.Float(node["value"])
	}

	var summary errorSummary
	byLogID := make(map[string]*datasetErrors)
	for _, item := range jsonvalue.List(raw, "groups_series") {
		group := jsonvalue.Map(item)
		if group == nil {
			continue
		}
		var isError bool
		switch name := strings.ToLower(strings.TrimSpace(jsonvalue.String(group["name"]))); {
		case strings.HasPrefix(name, "error"):
			isError = true
			summary.Errors = count(group)
		case strings.HasPrefix(name, "warn"):
			summary.Warnings = count(group)
		default:
			continue
		}

		for _, subItem := range jsonvalue.Slice(group["groups_series"]) {
			sub := jsonvalue.Map(subItem)
			if sub == nil || sub["name"] == nil {
				continue
			}
			logID := jsonvalue.String(sub["name"])
			entry, ok := byLogID[logID]
			if !ok {
				meta := index[logID]
				entry = &datasetErrors{LogID: logID, Name: meta["log"], Collection: meta["logset"]}
				byLogID[logID] = entry
				summary.ByDataset = append(summary.ByDataset, entry)
			}
			if isError {
				entry.Errors = count(sub)
			} else {
				entry.Warnings = count(sub)
			}
		}
	}

	slices.SortFunc(summary.ByDataset, func(a, b *datasetErrors) int {
		return cmp.Or(
			cmp.Compare(b.Errors+b.Warnings, a.Errors+a.Warnings),
			cmp.Compare(a.LogID, b.LogID),
		)
	})
	return summary
}

// usageByLog indexes one usage type's rows by dataset id.
func usageByLog(ctx context.Context, client *brontosdk.Client, usageType, timeRange string,
) (map[string]brontosdk.UsageRow, error) {
	rows, err := client.UsageByLog(ctx, usageType, timeRange)
	if err != nil {
		return nil, err
	}
	index := make(map[string]brontosdk.UsageRow, len(rows))
	for _, row := range rows {
		index[row.LogID] = row
	}
	return index, nil
}

type datasetUsage struct {
	LogID       string `json:"log_id"`
	IngestBytes int64  `json:"ingest_bytes"`
	SearchBytes int64  `json:"search_bytes"`
}

type usageSummary struct {
	DatasetsTotal int    `json:"datasets_total"`
	IngestBytes   int64  `json:"ingest_bytes"`
	SearchBytes   int64  `json:"search_bytes"`
	IngestLimit   *int64 `json:"ingest_limit,omitempty"`
	SearchLimit   *int64 `json:"search_limit,omitempty"`
}

// GetUsage reports ingestion and search usage against limits.
func GetUsage(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name: string(MethodGetUsage),
			Annotations: &mcp.ToolAnnotations{
				Title:           "Get Usage",
				ReadOnlyHint:    true,
				DestructiveHint: new(false),
				OpenWorldHint:   new(false),
			},
			Description: "Reports per-dataset ingestion and search usage (bytes) for the org against its monthly " +
				"limits. Use to understand data volume and spend. Only billable usage is reported: system " +
				"collections such as .usage and .audit-trail are excluded.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"time_range": timeRangeSchema(
						"Relative window, \"This month\" by default; limits are per calendar month."),
				},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			arguments, err := helpers.NewToolArguments(request)
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}
			timeRange := arguments.GetString("time_range", "")
			if timeRange == "" {
				timeRange = "This month"
			}

			ingest, err := usageByLog(ctx, client, brontosdk.UsageIngestion, timeRange)
			if err != nil {
				return errorResult(err, "")
			}
			search, err := usageByLog(ctx, client, brontosdk.UsageSearch, timeRange)
			if err != nil {
				return errorResult(err, "")
			}
			limits, err := client.Limits(ctx)
			if err != nil {
				return errorResult(err, "")
			}

			byLogID := make(map[string]*datasetUsage)
			for _, index := range []map[string]brontosdk.UsageRow{ingest, search} {
				for logID := range index {
					byLogID[logID] = &datasetUsage{
						LogID:       logID,
						IngestBytes: ingest[logID].BytesTotal,
						SearchBytes: search[logID].BytesTotal,
					}
				}
			}

			var summary usageSummary
			datasets := make([]*datasetUsage, 0, len(byLogID))
			for _, d := range byLogID {
				summary.IngestBytes += d.IngestBytes
				summary.SearchBytes += d.SearchBytes
				datasets = append(datasets, d)
			}
			summary.DatasetsTotal = len(datasets)
			slices.SortFunc(datasets, func(a, b *datasetUsage) int {
				return cmp.Or(cmp.Compare(b.IngestBytes, a.IngestBytes), cmp.Compare(a.LogID, b.LogID))
			})
			summary.IngestLimit, summary.SearchLimit = orgLimits(limits)

			return helpers.NewToolResultJSON(map[string]any{"summary": summary, "datasets": datasets})
		},
	}
}

// orgLimits reads the organisation's system limits: ingestion in bytes, and
// search as a ratio of the ingestion limit. Either is nil when not set.
func orgLimits(limits []any) (ingestLimit, searchLimit *int64) {
	var searchRatio any
	for _, item := range limits {
		limit := jsonvalue.Map(item)
		if limit["target"] != "ORGANISATION" || limit["type"] != "SYSTEM" {
			continue
		}
		switch {
		case limit["category"] == "INGESTION_LIMITS" && limit["unit"] == "BYTES":
			ingestLimit = new(jsonvalue.Int(limit["value"]))
		case limit["category"] == "SEARCH_LIMITS" && limit["unit"] == "RATIO":
			searchRatio = limit["value"]
		}
	}
	if ingestLimit != nil && searchRatio != nil {
		searchLimit = new(int64(jsonvalue.Float(searchRatio) * float64(*ingestLimit)))
	}
	return ingestLimit, searchLimit
}

type usageAxis struct {
	Active bool  `json:"active"`
	Bytes  int64 `json:"bytes"`
}

type ingestionAxis struct {
	Active          bool  `json:"active"`
	Bytes           int64 `json:"bytes"`
	LastHeartbeatAt any   `json:"last_heartbeat_at,omitempty"`
}

type datasetCoverage struct {
	LogID      string `json:"log_id"`
	Name       any    `json:"name"`
	Collection any    `json:"collection"`
	// Monitors are the ids of the monitors that read the dataset. Empty is a
	// blind spot.
	Monitors  []string      `json:"monitors"`
	Search    usageAxis     `json:"search"`
	Ingestion ingestionAxis `json:"ingestion"`
}

type coverageSummary struct {
	DatasetsTotal int `json:"datasets_total"`
	Monitored     int `json:"monitored"`
	Unmonitored   int `json:"unmonitored"`
}

// GetCoverage reports which datasets are monitored, searched and ingesting.
func GetCoverage(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name: string(MethodGetCoverage),
			Annotations: &mcp.ToolAnnotations{
				Title:           "Get Coverage",
				ReadOnlyHint:    true,
				DestructiveHint: new(false),
				OpenWorldHint:   new(false),
			},
			Description: "Reports, per dataset, whether it is monitored (which monitors cover it), searched, and " +
				"still ingesting — surfacing blind spots such as a live dataset that no monitor is watching. " +
				"Monitor coverage is resolved from the explicit dataset ids in monitors' backing queries; " +
				"monitors that select datasets with a from_expr are not resolved, so their datasets may appear " +
				"unmonitored even though a monitor covers them.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"time_range": timeRangeSchema(
						"Window for the searched and ingesting axes, \"This month\" by default."),
				},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			arguments, err := helpers.NewToolArguments(request)
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}
			timeRange := arguments.GetString("time_range", "")
			if timeRange == "" {
				timeRange = "This month"
			}

			datasets, err := client.Datasets(ctx)
			if err != nil {
				return errorResult(err, "")
			}
			monitorsByLog, err := monitorLogIDs(ctx, client)
			if err != nil {
				return errorResult(err, "")
			}
			ingest, err := usageByLog(ctx, client, brontosdk.UsageIngestion, timeRange)
			if err != nil {
				return errorResult(err, "")
			}
			search, err := usageByLog(ctx, client, brontosdk.UsageSearch, timeRange)
			if err != nil {
				return errorResult(err, "")
			}

			// The usage API omits non-billable system collections (.usage,
			// .audit-trail), so ingestion activity also falls back to the
			// dataset's last heartbeat.
			heartbeatCutoff := now().Add(-activeWindow).UnixMilli()

			var summary coverageSummary
			out := []datasetCoverage{}
			for _, item := range datasets {
				d := jsonvalue.Map(item)
				logID := jsonvalue.String(d["log_id"])
				if logID == "" {
					continue
				}
				heartbeat := jsonvalue.Map(d["metadata"])["last_heartbeat_at"]
				monitors := monitorsByLog[logID]
				if monitors == nil {
					monitors = []string{}
				}

				out = append(out, datasetCoverage{
					LogID:      logID,
					Name:       d["log"],
					Collection: d["logset"],
					Monitors:   monitors,
					Search: usageAxis{
						Active: search[logID].Count > 0,
						Bytes:  search[logID].BytesTotal,
					},
					Ingestion: ingestionAxis{
						Active: ingest[logID].Count > 0 ||
							(heartbeat != nil && jsonvalue.Int(heartbeat) >= heartbeatCutoff),
						Bytes:           ingest[logID].BytesTotal,
						LastHeartbeatAt: heartbeat,
					},
				})
				if len(monitors) > 0 {
					summary.Monitored++
				} else {
					summary.Unmonitored++
				}
			}
			summary.DatasetsTotal = len(out)

			return helpers.NewToolResultJSON(map[string]any{"summary": summary, "datasets": out})
		},
	}
}
