// Package bronto holds the Bronto toolset exposed by this server.
//
// The framework around the tools — toolset groups, input validation, argument
// binding, result helpers — comes from github.com/teamwork/mcp/pkg and is never
// re-implemented here. Requests to Bronto go through internal/brontosdk.
package bronto

import (
	"github.com/teamwork/bronto-mcp-go/internal/brontosdk"
	"github.com/teamwork/mcp/pkg/toolsets"
)

// Toolset keys, the valid values of the -toolsets flag besides "all".
const (
	// ToolsetSearch covers event and aggregate searches over log data.
	ToolsetSearch toolsets.Method = "bronto-search"
	// ToolsetDatasets covers dataset and key discovery.
	ToolsetDatasets toolsets.Method = "bronto-datasets"
	// ToolsetMonitors covers creating, listing and health-checking monitors.
	ToolsetMonitors toolsets.Method = "bronto-monitors"
	// ToolsetSavedSearches covers listing, creating and running saved searches.
	ToolsetSavedSearches toolsets.Method = "bronto-saved-searches"
	// ToolsetOrganisation covers org-wide error, usage and coverage reports.
	ToolsetOrganisation toolsets.Method = "bronto-org"
)

// Tool method names as exposed to MCP clients.
//
// They carry no namespace prefix: they are the names the Python server this
// one replaces exposed, and clients and tool descriptions refer to them as such.
const (
	// MethodSearchLogs searches log events.
	MethodSearchLogs toolsets.Method = "search_logs"
	// MethodTimeseries computes aggregate metrics over time.
	MethodTimeseries toolsets.Method = "timeseries"

	// MethodGetDatasets lists every dataset.
	MethodGetDatasets toolsets.Method = "get_datasets"
	// MethodGetDatasetsByName finds datasets by name and collection.
	MethodGetDatasetsByName toolsets.Method = "get_datasets_by_name"
	// MethodGetKeys lists the keys present in one dataset.
	MethodGetKeys toolsets.Method = "get_keys"
	// MethodGetAllDatasetsKeys lists the keys present in every dataset.
	MethodGetAllDatasetsKeys toolsets.Method = "get_all_datasets_keys"
	// MethodGetKeyValues lists sample values of one key in one dataset.
	MethodGetKeyValues toolsets.Method = "get_key_values"

	// MethodCreateMonitor creates a muted monitor.
	MethodCreateMonitor toolsets.Method = "create_monitor"
	// MethodSearchMonitors lists or inspects monitors.
	MethodSearchMonitors toolsets.Method = "search_monitors"
	// MethodValidateMonitors health-checks every monitor.
	MethodValidateMonitors toolsets.Method = "validate_monitors"

	// MethodGetSavedSearches lists or fetches saved searches.
	MethodGetSavedSearches toolsets.Method = "get_saved_searches"
	// MethodCreateSavedSearch creates a saved search.
	MethodCreateSavedSearch toolsets.Method = "create_saved_search"
	// MethodRunSavedSearch runs a saved search.
	MethodRunSavedSearch toolsets.Method = "run_saved_search"

	// MethodGetErrorSummary summarises parsing and ingestion errors.
	MethodGetErrorSummary toolsets.Method = "get_error_summary"
	// MethodGetUsage reports ingestion and search usage against limits.
	MethodGetUsage toolsets.Method = "get_usage"
	// MethodGetCoverage reports which datasets are monitored, searched and
	// ingesting.
	MethodGetCoverage toolsets.Method = "get_coverage"
)

const (
	searchDescription = "Search log events and compute aggregate metrics over Bronto log data."

	datasetsDescription = "Discover Bronto datasets, the collections they belong to, and the keys and " +
		"values their data contains."

	monitorsDescription = "Create, list and health-check Bronto alerting monitors."

	savedSearchesDescription = "List, create and run Bronto saved searches."

	organisationDescription = "Report org-wide parsing errors, ingestion and search usage, and " +
		"monitoring coverage."
)

// Instructions tell a client how the tools fit together. They are sent in the
// initialize handshake.
const Instructions = "Use this MCP server to interact with log data stored in Bronto as well as its metadata, " +
	"datasets, collections, keys, etc. The tools provided by this server help\n" +
	"- selecting datasets based on their tags or the keys that their data contains. For instance,\n" +
	"  - when looking to select datasets based on the key they contain, the following process should be used:\n" +
	"    - list the datasets and check their tags. Select datasets whose tags match the keys under interest.\n" +
	"    - also list the keys of all datasets and select datasets that contain the key under interest.\n" +
	"  - when looking to select datasets based on the value of a key that they contain, then the following " +
	"process should be used:\n" +
	"    - list the datasets and check their tags. Select datasets whose tags and values match the keys and " +
	"values under interest.\n" +
	"    - also always list the keys of all datasets to look for keys that are relevant to the one under " +
	"interest. Then retrieve the dataset IDs for datasets that contain at least one of those keys and so " +
	"that the value of the key matches the provided value.\n" +
	"- searching log events in datasets, based on some filter. Filters are typically based on some of the " +
	"keys and values that the dataset contains.\n" +
	"- computing metrics present in datasets, based on some filter, and grouping the results according to " +
	"some of the keys present in the dataset."

func init() {
	toolsets.RegisterMethod(ToolsetSearch)
	toolsets.RegisterMethod(ToolsetDatasets)
	toolsets.RegisterMethod(ToolsetMonitors)
	toolsets.RegisterMethod(ToolsetSavedSearches)
	toolsets.RegisterMethod(ToolsetOrganisation)

	// tools/list keeps the order the Python server registered them in, so
	// clients that truncate the list keep the search and discovery tools.
	toolsets.RegisterToolOrder([]toolsets.Method{
		MethodSearchLogs,
		MethodTimeseries,
		MethodGetDatasets,
		MethodGetDatasetsByName,
		MethodGetKeys,
		MethodGetAllDatasetsKeys,
		MethodGetKeyValues,
		MethodCreateMonitor,
		MethodGetSavedSearches,
		MethodCreateSavedSearch,
		MethodRunSavedSearch,
		MethodSearchMonitors,
		MethodGetErrorSummary,
		MethodGetUsage,
		MethodGetCoverage,
		MethodValidateMonitors,
	})
}

// DefaultToolsetGroup builds the toolset group this server registers. With
// readOnly set, the tools that create monitors and saved searches are left out.
//
// The group declares no namespace. That is what scopes tools/list to an OAuth
// token's grants, and this server takes no OAuth tokens: every caller acts
// with the one Bronto API key the server was started with.
func DefaultToolsetGroup(readOnly bool, client *brontosdk.Client) *toolsets.ToolsetGroup {
	group := toolsets.NewToolsetGroup(readOnly)

	group.AddToolset(toolsets.NewToolset(ToolsetSearch, searchDescription).
		AddReadTools(
			SearchLogs(client),
			Timeseries(client),
		),
	)

	group.AddToolset(toolsets.NewToolset(ToolsetDatasets, datasetsDescription).
		AddReadTools(
			GetDatasets(client),
			GetDatasetsByName(client),
			GetKeys(client),
			GetAllDatasetsKeys(client),
			GetKeyValues(client),
		),
	)

	group.AddToolset(toolsets.NewToolset(ToolsetMonitors, monitorsDescription).
		AddReadTools(
			SearchMonitors(client),
			ValidateMonitors(client),
		).
		AddWriteTools(
			CreateMonitor(client),
		),
	)

	group.AddToolset(toolsets.NewToolset(ToolsetSavedSearches, savedSearchesDescription).
		AddReadTools(
			GetSavedSearches(client),
			RunSavedSearch(client),
		).
		AddWriteTools(
			CreateSavedSearch(client),
		),
	)

	group.AddToolset(toolsets.NewToolset(ToolsetOrganisation, organisationDescription).
		AddReadTools(
			GetErrorSummary(client),
			GetUsage(client),
			GetCoverage(client),
		),
	)

	return group
}
