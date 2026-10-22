// Command bronto-mcp-http serves the Bronto MCP tools over streamable HTTP.
//
// Everything except the toolset itself comes from github.com/teamwork/mcp/pkg:
// configuration and startup, the MCP server and its middleware, the health
// endpoint. Keep it that way — a second copy of any of it drifts from the
// original.
//
// Unlike the Teamwork.com servers, this one takes no OAuth tokens: it calls
// Bronto with the single API key it was started with, so it has no bearer
// authentication and no protected-resource metadata. Run it where only the
// people entitled to that key can reach it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teamwork/bronto-mcp-go/internal/bronto"
	"github.com/teamwork/bronto-mcp-go/internal/brontosdk"
	"github.com/teamwork/mcp/pkg/cli"
	"github.com/teamwork/mcp/pkg/config"
	"github.com/teamwork/mcp/pkg/mcphttp"
	"github.com/teamwork/mcp/pkg/toolsets"
)

const (
	serverName  = "Bronto"
	serverTitle = "Bronto Model Context Protocol"

	// envPrefix namespaces the server configuration pkg/config reads, so
	// BRONTO_MCP_SERVER_ADDRESS, BRONTO_MCP_LOG_LEVEL and friends.
	envPrefix = "BRONTO_MCP_"

	// defaultMCPURL is only reported, never used for authorisation: this server
	// publishes no OAuth metadata. BRONTO_MCP_URL overrides it.
	defaultMCPURL = "http://localhost:8080"

	// The Bronto API is configured by the same variables the Python server read.
	envAPIKey   = "BRONTO_API_KEY"
	envEndpoint = "BRONTO_API_ENDPOINT"

	// envFile holds local configuration, read from the working directory.
	// Variables already set in the environment take precedence over it.
	envFile = ".env"

	// brontoTimeout bounds one Bronto API call. Searches over long ranges are
	// the slowest thing the tools do.
	brontoTimeout = 2 * time.Minute

	shutdownTimeout = 5 * time.Second
)

var methods = cli.NewMethods(toolsets.MethodAll)

// quietPaths are endpoints not worth a log line or a trace: health checks fire
// constantly and browsers probe the favicon.
var quietPaths = map[string]struct{}{
	"/favicon.ico": {},
	"/api/health":  {},
}

func main() {
	defer handleExit()

	flag.Var(methods, "toolsets", "Comma-separated list of toolsets to enable")
	readOnly := flag.Bool("read-only", false, "Only expose tools that do not write")
	flag.Parse()

	// Read before config.Load, which reads the BRONTO_MCP_ variables. A
	// missing file is fine: deployments configure the environment directly.
	if err := godotenv.Load(envFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "failed to read %s: %v\n", envFile, err)
		exit(exitCodeSetupFailure)
	}

	resources, teardown := config.Load(os.Stdout,
		config.WithEnvPrefix(envPrefix),
		config.WithServerIdentity(serverName, serverTitle),
		config.WithDefaultMCPURL(defaultMCPURL),
	)
	defer teardown()

	apiKey := strings.TrimSpace(os.Getenv(envAPIKey))
	if apiKey == "" {
		resources.Logger().Error("missing Bronto API key", slog.String("variable", envAPIKey))
		exit(exitCodeSetupFailure)
	}
	endpoint := strings.TrimSpace(os.Getenv(envEndpoint))
	if endpoint == "" {
		endpoint = brontosdk.DefaultEndpoint
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// The Bronto client gets its own HTTP client rather than the shared
	// Teamwork one: that one's logging transport writes out every request
	// header but Authorization, and the API key travels in x-bronto-api-key.
	client := brontosdk.NewClient(endpoint, apiKey, &http.Client{Timeout: brontoTimeout})

	group := bronto.DefaultToolsetGroup(*readOnly, client)
	if err := group.EnableToolsets(methods.Toolsets()...); err != nil {
		resources.Logger().Error("failed to enable toolsets", slog.String("error", err.Error()))
		exit(exitCodeSetupFailure)
	}
	mcpServer := config.NewMCPServer(resources, group)
	mcpServer.AddReceivingMiddleware(withInstructions(bronto.Instructions))

	mux := http.NewServeMux()
	mcphttp.Health(mux, "/api/health")
	mux.Handle("/", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
		// Pinned to the limit mcphttp.LimitBody enforces. Left at zero the SDK
		// applies its own smaller default, tightening the limit silently.
		MaxRequestBodyBytes: mcphttp.DefaultMaxBodySize,
	}))

	httpServer := &http.Server{
		Addr:              resources.Info.ServerAddress,
		Handler:           addRouterMiddlewares(resources, mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	resources.Logger().Info("starting http server",
		slog.String("address", resources.Info.ServerAddress),
		slog.String("bronto_endpoint", endpoint),
		slog.String("env_prefix", envPrefix),
	)
	// ListenAndServe only returns ErrServerClosed once Shutdown is called,
	// which happens below, so anything it sends here is a real failure.
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.ListenAndServe() }()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		resources.Logger().Error("failed to start server",
			slog.String("address", resources.Info.ServerAddress),
			slog.String("error", err.Error()),
		)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		resources.Logger().Error("server shutdown failed", slog.String("error", err.Error()))
	}
	resources.Logger().Info("server stopped")
}

// withInstructions adds the server's usage instructions to the initialize
// handshake. config.NewMCPServer builds the server options itself and leaves
// them empty.
func withInstructions(instructions string) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if initialize, ok := result.(*mcp.InitializeResult); ok && err == nil {
				initialize.Instructions = instructions
			}
			return result, err
		}
	}
}

func addRouterMiddlewares(resources config.Resources, mux *http.ServeMux) http.Handler {
	return mcphttp.Chain(mux,
		func(h http.Handler) http.Handler { return mcphttp.LimitBody(mcphttp.DefaultMaxBodySize, h) },
		mcphttp.RequestInfo,
		func(h http.Handler) http.Handler { return mcphttp.Log(resources.Logger(), quietPaths, h) },
		func(h http.Handler) http.Handler { return mcphttp.Sentry(resources, h) },
		func(h http.Handler) http.Handler { return mcphttp.Tracer(resources, quietPaths, h) },
	)
}

type exitCode int

const (
	exitCodeOK exitCode = iota
	exitCodeSetupFailure
)

type exitData struct {
	code exitCode
}

// exit aborts the program while still running every deferred call, which
// os.Exit would skip — teardown flushes Sentry and stops the tracer.
func exit(code exitCode) {
	panic(exitData{code: code})
}

// handleExit turns the panic from exit back into a process exit code, and
// re-panics anything else.
func handleExit() {
	if e := recover(); e != nil {
		if exit, ok := e.(exitData); ok {
			os.Exit(int(exit.code))
		}
		panic(e)
	}
}
