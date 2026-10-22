package bronto

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teamwork/bronto-mcp-go/internal/brontosdk"
	"github.com/teamwork/bronto-mcp-go/internal/jsonvalue"
	"github.com/teamwork/mcp/pkg/helpers"
	"github.com/teamwork/mcp/pkg/toolsets"
	"golang.org/x/sync/errgroup"
)

// comparisonOperators are how a monitor compares its metric to the threshold.
var comparisonOperators = helpers.SliceToAny([]string{"ABOVE", "BELOW", "ABOVE_OR_EQUAL", "BELOW_OR_EQUAL"})

// healthCheckWindow is how far back validate_monitors looks for data.
const healthCheckWindow = "Last 24 hours"

// healthCheckConcurrency caps how many health-check searches validate_monitors
// runs against Bronto at once.
const healthCheckConcurrency = 8

type monitorQuery struct {
	Name   string   `json:"name"`
	Select []string `json:"select"`
	From   []string `json:"from"`
	Where  string   `json:"where,omitempty"`
}

type monitorAction struct {
	Type  string `json:"type"`
	Email string `json:"email"`
}

// monitorBody is what POST /monitors takes.
type monitorBody struct {
	Name               string          `json:"name"`
	MonitorType        string          `json:"monitor_type"`
	Type               string          `json:"type"`
	Threshold          float64         `json:"threshold"`
	ComparisonOperator string          `json:"comparison_operator"`
	Window             string          `json:"window"`
	Queries            []monitorQuery  `json:"queries"`
	MuteUntil          int             `json:"mute_until"`
	Actions            []monitorAction `json:"actions"`
}

// mutedIndefinitely is the mute_until that keeps a monitor muted until someone
// unmutes it.
const mutedIndefinitely = -1

// CreateMonitor creates a muted monitor.
func CreateMonitor(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name:        string(MethodCreateMonitor),
			Annotations: writeAnnotations("Create Monitor"),
			Description: "Creates an alerting monitor over one or more datasets. The monitor is ALWAYS created " +
				"muted so it can be reviewed in the Bronto UI before it can alert. A notification email is " +
				"required and MUST belong to an existing Bronto user (ask the user for it); the monitor " +
				"notifies that address. Provide the threshold, comparison operator, evaluation window, the " +
				"dataset log IDs, and an aggregate select such as count(*). Returns the created monitor, " +
				"including its id. With dry_run, returns {dry_run: true, monitor_body} and creates nothing.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "The monitor name.",
					},
					"notification_email": {
						Type: "string",
						Description: "Email address to notify when the monitor fires. MUST be the email of " +
							"an existing Bronto user — ask the user for it if you do not have it.",
					},
					"threshold": {
						Type:        "number",
						Description: "The alarm threshold for the monitor.",
					},
					"comparison_operator": {
						Type:        "string",
						Description: "How the metric is compared to the threshold.",
						Enum:        comparisonOperators,
					},
					"window": {
						Type:        "string",
						Description: "Evaluation window, e.g. \"Last 15 minutes\".",
					},
					"log_ids": logIDsSchema("Dataset IDs the monitor evaluates over."),
					"select": optional(&jsonschema.Schema{
						Type: "string",
						Description: "Aggregate the monitor evaluates, e.g. count(*) or avg(duration_ms). " +
							"Defaults to count(*).",
					}),
					"search_filter": optional(&jsonschema.Schema{
						Type:        "string",
						Description: "Optional SQL-style WHERE filter applied before aggregation.",
					}),
					"dry_run": optional(&jsonschema.Schema{
						Type:        "boolean",
						Description: "Validate and return the resolved monitor body without creating anything.",
					}),
				},
				Required: []string{
					"name", "notification_email", "threshold", "comparison_operator", "window", "log_ids",
				},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var name, email, operator, window string
			var threshold float64
			var logIDs []string
			arguments, err := bindArguments(request,
				helpers.RequiredParam(&name, "name"),
				helpers.RequiredParam(&email, "notification_email"),
				helpers.RequiredNumericParam(&threshold, "threshold"),
				helpers.RequiredParam(&operator, "comparison_operator"),
				helpers.RequiredParam(&window, "window"),
				requiredStrings(&logIDs, "log_ids"),
			)
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}

			// Only an existing Bronto user's email can receive monitor notifications.
			users, err := client.Users(ctx)
			if err != nil {
				return errorResult(err, "")
			}
			if !hasUserEmail(users, email) {
				return helpers.NewToolResultTextError(
					"notification_email '%s' is not an existing Bronto user. Ask the user for a valid account email.",
					email,
				), nil
			}

			body := monitorBody{
				Name:               name,
				MonitorType:        "PATTERN",
				Type:               "PATTERN",
				Threshold:          threshold,
				ComparisonOperator: operator,
				Window:             window,
				Queries: []monitorQuery{{
					Name:   name,
					Select: []string{stringOr(arguments, "select", countAll)},
					From:   logIDs,
					Where:  arguments.GetString("search_filter", ""),
				}},
				MuteUntil: mutedIndefinitely,
				Actions:   []monitorAction{{Type: "EMAIL", Email: email}},
			}
			if arguments.GetBool("dry_run", false) {
				return helpers.NewToolResultJSON(map[string]any{"dry_run": true, "monitor_body": body})
			}

			created, err := client.CreateMonitor(ctx, body)
			if err != nil {
				return errorResult(err, "")
			}
			return helpers.NewToolResultJSON(created)
		},
	}
}

// hasUserEmail reports whether email belongs to one of users, ignoring case
// and surrounding space.
func hasUserEmail(users []any, email string) bool {
	email = strings.TrimSpace(email)
	for _, user := range users {
		candidate := strings.TrimSpace(jsonvalue.String(jsonvalue.Map(user)["email"]))
		if candidate != "" && strings.EqualFold(candidate, email) {
			return true
		}
	}
	return false
}

// SearchMonitors lists or inspects monitors.
func SearchMonitors(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name:        string(MethodSearchMonitors),
			Annotations: readAnnotations("Search Monitors"),
			Description: "Lists monitors, optionally filtered by a text query or status. With a monitor_id, " +
				"returns that monitor plus its recent trigger/recovery events.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"query": optional(&jsonschema.Schema{
						Type:        "string",
						Description: "Optional case-insensitive text filter.",
					}),
					"status": optional(&jsonschema.Schema{
						Type:        "string",
						Description: "Optional exact status filter, e.g. OK, ALARM.",
					}),
					"monitor_id": optional(&jsonschema.Schema{
						Type:        "string",
						Description: "Inspect one monitor and its recent events.",
					}),
				},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			arguments, err := helpers.NewToolArguments(request)
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}

			monitors, err := client.Monitors(ctx)
			if err != nil {
				return errorResult(err, "")
			}

			if id := arguments.GetString("monitor_id", ""); id != "" {
				for _, item := range monitors {
					if monitorID(jsonvalue.Map(item)) != id {
						continue
					}
					events, err := client.MonitorEvents(ctx, id)
					if err != nil {
						return errorResult(err, "")
					}
					if events == nil {
						events = []any{}
					}
					return helpers.NewToolResultJSON(map[string]any{"monitor": item, "events": events})
				}
				return helpers.NewToolResultJSON(map[string]any{"monitor": nil, "events": []any{}})
			}

			status := strings.ToLower(arguments.GetString("status", ""))
			query := strings.ToLower(arguments.GetString("query", ""))
			matches := []any{}
			for _, item := range monitors {
				monitor := jsonvalue.Map(item)
				if status != "" {
					current := jsonvalue.String(jsonvalue.FirstTruthy(monitor["status"], monitor["state"]))
					if strings.ToLower(current) != status {
						continue
					}
				}
				if query != "" && !strings.Contains(strings.ToLower(jsonvalue.Text(item)), query) {
					continue
				}
				matches = append(matches, item)
			}
			return helpers.NewToolResultJSON(map[string]any{"monitors": matches, "total": len(matches)})
		},
	}
}

// monitorLogIDs maps each dataset id to the ids of the monitors whose backing
// query reads it.
//
// Only a query's explicit "from" is resolved. A query that selects datasets
// with a from_expr would need a catalogue lookup, so its monitor contributes
// no dataset ids.
func monitorLogIDs(ctx context.Context, client *brontosdk.Client) (map[string][]string, error) {
	monitors, byID, err := monitorsAndDefinitions(ctx, client)
	if err != nil {
		return nil, err
	}

	idsByLog := make(map[string][]string)
	for _, item := range monitors {
		monitor := jsonvalue.Map(item)
		id := monitorID(monitor)
		metricID := jsonvalue.String(monitor["metric_id"])
		if id == "" || metricID == "" {
			continue
		}
		for _, query := range jsonvalue.Slice(byID[metricID]["queries"]) {
			for _, logID := range fromLogIDs(jsonvalue.Map(query)["from"]) {
				idsByLog[logID] = append(idsByLog[logID], id)
			}
		}
	}
	return idsByLog, nil
}

// monitorsAndDefinitions fetches every monitor along with the metric
// definitions that back them, indexed by id. A monitor's metric_id names its
// definition.
func monitorsAndDefinitions(ctx context.Context, client *brontosdk.Client,
) (monitors []any, definitions map[string]map[string]any, err error) {
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		monitors, err = client.Monitors(ctx)
		return err
	})
	group.Go(func() error {
		list, err := client.MetricDefinitions(ctx)
		definitions = indexBy(list, "id")
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, nil, err
	}
	return monitors, definitions, nil
}

// Health statuses a monitor can be given by validate_monitors.
const (
	healthOK    = "ok"
	healthWarn  = "warn"
	healthError = "error"
)

type monitorHealth struct {
	MonitorID string `json:"monitor_id"`
	Name      any    `json:"name"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
}

type healthSummary struct {
	OK    int `json:"ok"`
	Warn  int `json:"warn"`
	Error int `json:"error"`
}

func (s *healthSummary) add(status string) {
	switch status {
	case healthOK:
		s.OK++
	case healthWarn:
		s.Warn++
	default:
		s.Error++
	}
}

// ValidateMonitors health-checks every monitor.
func ValidateMonitors(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name:        string(MethodValidateMonitors),
			Annotations: readAnnotations("Validate Monitors"),
			Description: "Health-checks existing monitors by re-running each monitor's backing query over the " +
				"last 24 hours: 'error' = the monitor cannot fire as built, 'warn' = valid but no data, 'ok' = " +
				"data present. Use to find alarms that have silently broken or gone quiet.",
			InputSchema: noArgumentsSchema(),
		},
		Handler: func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			monitors, byID, err := monitorsAndDefinitions(ctx, client)
			if err != nil {
				return errorResult(err, "")
			}

			// Monitors sharing a metric definition share its query, so each
			// definition is checked once, several at a time.
			checks := make(map[string]*monitorHealth)
			for _, item := range monitors {
				checks[jsonvalue.String(jsonvalue.Map(item)["metric_id"])] = &monitorHealth{}
			}
			group, groupCtx := errgroup.WithContext(ctx)
			group.SetLimit(healthCheckConcurrency)
			for metricID, check := range checks {
				group.Go(func() error {
					var err error
					check.Status, check.Reason, err = checkMonitorQuery(groupCtx, client, firstQuery(byID[metricID]))
					return err
				})
			}
			if err := group.Wait(); err != nil {
				// Bronto could not be reached at all, so no other monitor can be
				// checked either.
				return errorResult(err, "")
			}

			var summary healthSummary
			results := make([]monitorHealth, 0, len(monitors))
			for _, item := range monitors {
				monitor := jsonvalue.Map(item)
				check := checks[jsonvalue.String(monitor["metric_id"])]
				summary.add(check.Status)
				results = append(results, monitorHealth{
					MonitorID: monitorID(monitor),
					Name:      monitor["name"],
					Status:    check.Status,
					Reason:    check.Reason,
				})
			}
			return helpers.NewToolResultJSON(map[string]any{"summary": summary, "results": results})
		},
	}
}

// firstQuery returns the first query of a metric definition, or nil.
func firstQuery(definition map[string]any) map[string]any {
	for _, query := range jsonvalue.Slice(definition["queries"]) {
		if query, ok := query.(map[string]any); ok {
			return query
		}
	}
	return nil
}

// checkMonitorQuery counts what a monitor's backing query matched over the
// health-check window. Bronto rejecting the query is a finding about the
// monitor; only failing to reach Bronto is returned as an error.
func checkMonitorQuery(
	ctx context.Context,
	client *brontosdk.Client,
	query map[string]any,
) (status, reason string, err error) {
	logIDs := fromLogIDs(query["from"])
	fromExpr := query["from_expr"]
	if len(logIDs) == 0 && !jsonvalue.Truthy(fromExpr) {
		return healthError, "monitor has no resolvable backing query", nil
	}

	body := map[string]any{
		"select":     []string{countAll},
		"time_range": healthCheckWindow,
	}
	if len(logIDs) > 0 {
		body["from"] = logIDs
	} else {
		body["from_expr"] = fromExpr
	}
	if where := strings.TrimSpace(jsonvalue.String(query["where"])); where != "" {
		body["where"] = query["where"]
	}

	resp, err := client.Search(ctx, body)
	if err != nil {
		if isBrontoAnswer(err) {
			return healthError, fmt.Sprintf("query failed: %v", err), nil
		}
		return "", "", err
	}
	if jsonvalue.Int(jsonvalue.Map(resp["totals"])["count"]) > 0 {
		return healthOK, "data present", nil
	}
	return healthWarn, "query valid but no data in the last 24 hours", nil
}
