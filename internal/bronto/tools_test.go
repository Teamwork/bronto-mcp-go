package bronto

import (
	"slices"
	"testing"

	"github.com/teamwork/bronto-mcp-go/internal/brontosdk"
	"github.com/teamwork/mcp/pkg/toolsets"
)

// allTools is every tool the group registers, read and write.
func allTools(readOnly bool) []toolsets.ToolWrapper {
	group := DefaultToolsetGroup(readOnly, brontosdk.NewClient("", "", nil))
	if err := group.EnableToolsets(toolsets.MethodAll); err != nil {
		panic(err)
	}
	var tools []toolsets.ToolWrapper
	for _, toolset := range group.Toolsets {
		tools = append(tools, toolset.GetActiveTools()...)
	}
	return tools
}

func toolNames(tools []toolsets.ToolWrapper) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Tool.Name)
	}
	slices.Sort(names)
	return names
}

// The tool names are the Python server's: clients and descriptions refer to
// them, so renaming one is a breaking change.
func TestToolNamesMatchThePythonServer(t *testing.T) {
	want := []string{
		"create_monitor", "create_saved_search", "get_all_datasets_keys", "get_coverage", "get_datasets",
		"get_datasets_by_name", "get_error_summary", "get_key_values", "get_keys", "get_saved_searches",
		"get_usage", "run_saved_search", "search_logs", "search_monitors", "timeseries", "validate_monitors",
	}
	if got := toolNames(allTools(false)); !slices.Equal(got, want) {
		t.Errorf("tools = %v, want %v", got, want)
	}
}

func TestReadOnlyLeavesOutWriteTools(t *testing.T) {
	got := toolNames(allTools(true))
	for _, write := range []string{string(MethodCreateMonitor), string(MethodCreateSavedSearch)} {
		if slices.Contains(got, write) {
			t.Errorf("read-only group exposes %s", write)
		}
	}
	if len(got) != 14 {
		t.Errorf("read-only group has %d tools, want 14: %v", len(got), got)
	}
}

// DestructiveHint and OpenWorldHint are pointers: left nil, they are omitted
// from tools/list and the spec defaults both to true.
func TestAnnotationHintsAreExplicit(t *testing.T) {
	for _, tool := range allTools(false) {
		annotations := tool.Tool.Annotations
		if annotations == nil {
			t.Errorf("%s has no annotations", tool.Tool.Name)
			continue
		}
		if annotations.DestructiveHint == nil || annotations.OpenWorldHint == nil {
			t.Errorf("%s leaves DestructiveHint or OpenWorldHint unset", tool.Tool.Name)
		}
		if annotations.Title == "" {
			t.Errorf("%s has no title", tool.Tool.Name)
		}
	}
}

// tools/list keeps the Python server's order, so clients that truncate the
// list keep the search and discovery tools.
func TestToolOrderMatchesThePythonServer(t *testing.T) {
	want := []toolsets.Method{
		MethodSearchLogs, MethodTimeseries, MethodGetDatasets, MethodGetDatasetsByName, MethodGetKeys,
		MethodGetAllDatasetsKeys, MethodGetKeyValues, MethodCreateMonitor, MethodGetSavedSearches,
		MethodCreateSavedSearch, MethodRunSavedSearch, MethodSearchMonitors, MethodGetErrorSummary,
		MethodGetUsage, MethodGetCoverage, MethodValidateMonitors,
	}
	if got := toolsets.ToolOrder(); !slices.Equal(got, want) {
		t.Errorf("tool order = %v, want %v", got, want)
	}
}
