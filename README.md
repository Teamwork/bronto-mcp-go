# Bronto MCP Server (Go)

An [MCP](https://modelcontextprotocol.io) server that gives AI agents access to log data stored in
[Bronto](https://bronto.io): dataset discovery, event and aggregate searches, monitors, saved searches, and
org-wide error, usage and coverage reports.

It is a Go rewrite of the Python `bronto-mcp-server`. Tool names, parameters and result shapes are unchanged,
so agents and prompts written against the Python server keep working. It is built on the same stack as
`mcp-internal`: the official [Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk) and the shared
`github.com/teamwork/mcp/pkg` framework (toolsets, input validation, logging, Sentry and Datadog).

With these tools an agent can answer questions such as:

> _Can you please provide some log events from datasets in the ingestion collection, except for the ones related
> to garbage collection? The data should be between "2025-05-10 16:05:45" and "2025-05-10 16:25:05"._

## Capabilities

Sixteen tools in five toolsets. Write tools are marked ✏️ and are left out with `-read-only`.

### Search (`bronto-search`)

| Tool | What it does |
|---|---|
| `search_logs` | Searches log events in one or more datasets with an optional SQL-style `WHERE` filter. Bare words match full text; `"field"='value'` matches one field. Returns each event's raw message and its attributes. Defaults to the last 20 minutes. |
| `timeseries` | Computes aggregates (`AVG`, `MIN`, `MAX`, `COUNT`, `MEAN`, `MEDIAN`, `SUM`) over a time range, optionally grouped by keys. Returns, per group, a total event count and a series of data points. |

### Datasets (`bronto-datasets`)

| Tool | What it does |
|---|---|
| `get_datasets` | Lists every dataset with its name, collection, log ID and tags. |
| `get_datasets_by_name` | Finds the dataset with a given name in a given collection. |
| `get_keys` | Lists the keys present in one dataset. |
| `get_all_datasets_keys` | Maps every dataset ID to the keys present in it, to find datasets that carry a key. |
| `get_key_values` | Lists sample values of one key in one dataset. |

### Monitors (`bronto-monitors`)

| Tool | What it does |
|---|---|
| ✏️ `create_monitor` | Creates an alerting monitor over one or more datasets. It is **always created muted** for review in the Bronto UI, and its notification email must belong to an existing Bronto user. `dry_run` returns the monitor it would create without creating it. |
| `search_monitors` | Lists monitors, filtered by text or status, or returns one monitor with its recent trigger and recovery events. |
| `validate_monitors` | Re-runs every monitor's backing query over the last 24 hours and reports each as `ok` (data present), `warn` (valid, no data) or `error` (cannot fire as built). |

### Saved searches (`bronto-saved-searches`)

| Tool | What it does |
|---|---|
| `get_saved_searches` | Lists saved searches, filtered by text and limited in number, or fetches one by ID. |
| ✏️ `create_saved_search` | Saves a reusable search over datasets with an optional filter, select and time range. Supports `dry_run`. |
| `run_saved_search` | Runs a saved search, optionally over a different time range, and returns its events or aggregate results. |

### Organisation (`bronto-org`)

| Tool | What it does |
|---|---|
| `get_error_summary` | Org-wide parsing and ingestion error and warning totals, with a per-dataset breakdown. Precomputed, so it costs no search quota. |
| `get_usage` | Per-dataset ingestion and search bytes, with org totals against the monthly limits. Billable usage only. |
| `get_coverage` | Per dataset: which monitors watch it, whether it is searched, and whether it is still ingesting. Surfaces blind spots such as a live dataset no monitor watches. Monitors that select datasets with a `from_expr` are not resolved, so their datasets may show as unmonitored. |

The server also sends the Python server's usage instructions in the MCP `initialize` handshake. They tell an
agent how to pick datasets by their tags and keys before it searches them.

## Configuration

Bronto is configured with the same variables as the Python server:

| Variable | Default | |
|---|---|---|
| `BRONTO_API_KEY` | — | **Required.** The server will not start without it. |
| `BRONTO_API_ENDPOINT` | `https://api.eu.bronto.io` | The Bronto API, e.g. `https://api.eu.staging.bronto.io`. |

The server itself is configured by `github.com/teamwork/mcp/pkg/config` under the `BRONTO_MCP_` prefix:

| Variable | Default | |
|---|---|---|
| `BRONTO_MCP_SERVER_ADDRESS` | `:8080` | Address to listen on. |
| `BRONTO_MCP_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. |
| `BRONTO_MCP_LOG_FORMAT` | `text` | `text` or `json`. |
| `BRONTO_MCP_SENTRY_DSN` | — | Reports errors to Sentry when set. |
| `BRONTO_MCP_ENV` | `dev` | Environment name used in logs and traces. |

Datadog APM uses the standard agent variables (`DD_APM_TRACING_ENABLED`, `DD_SERVICE`, `DD_AGENT_HOST`, …).

### Flags

| Flag | |
|---|---|
| `-toolsets` | Comma-separated toolsets to enable, e.g. `bronto-search,bronto-datasets`. Defaults to `all`. |
| `-read-only` | Leave out `create_monitor` and `create_saved_search`. |

## Usage

Go 1.27 or newer.

```shell
make build
BRONTO_API_KEY=<API KEY HERE> \
BRONTO_API_ENDPOINT=https://api.eu.bronto.io \
./bin/bronto-mcp-http
```

or, without building first, `BRONTO_API_KEY=… make run`. The MCP endpoint is served at `/` over streamable HTTP
(stateless, JSON responses), and a health check at `/api/health`.

To use it from Claude Code:

```shell
claude mcp add --transport http bronto http://localhost:8080
```

This makes the server available to the current project. Add `--scope user` to make it available everywhere.
Then run `/mcp` in Claude Code to check that it connected. See the
[Claude Code MCP documentation](https://docs.anthropic.com/en/docs/claude-code/mcp) for more.

> The Python server listened on port 8000. Set `BRONTO_MCP_SERVER_ADDRESS=:8000` to keep an existing
> `claude mcp add` configuration working.

### Docker

```shell
make image
docker run --rm -p 8080:8080 -e BRONTO_API_KEY=<API KEY HERE> bronto-mcp:dev
```

## Security

The server has no authentication of its own. Every caller acts with the single Bronto API key the server was
started with, so only run it where the people who can reach it are entitled to that key — on your own machine,
or behind your network's access controls.

The API key travels in the `x-bronto-api-key` header. Bronto requests use their own HTTP client, not the shared
logging one, so the key is never written to the logs.

## Development

```shell
make test   # go test ./...
make lint   # golangci-lint, with the same configuration as mcp-internal
make fmt    # gofmt and go vet
```

| Path | |
|---|---|
| `cmd/bronto-mcp-http` | The HTTP server: configuration, middleware, startup and shutdown. |
| `internal/bronto` | The tools: schemas, argument handling, result shaping. One file per toolset. |
| `internal/brontosdk` | A small Bronto API client. It knows nothing about MCP. |
| `internal/jsonvalue` | Forgiving reads of loosely typed JSON from the Bronto API. |

To add a tool, add the API call to `internal/brontosdk`, the tool to `internal/bronto`, its name to the
`Method*` constants and the tool order in `tools.go`, and register it in `DefaultToolsetGroup` with
`AddReadTools` or `AddWriteTools`. Tool tests run against the fake Bronto API in `internal/bronto/main_test.go`.
