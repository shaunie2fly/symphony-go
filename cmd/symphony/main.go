package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/symphony-go/symphony/internal/agent"
	"github.com/symphony-go/symphony/internal/cmux"
	"github.com/symphony-go/symphony/internal/config"
	"github.com/symphony-go/symphony/internal/logging"
	"github.com/symphony-go/symphony/internal/orchestrator"
	"github.com/symphony-go/symphony/internal/server"
	"github.com/symphony-go/symphony/internal/tracker"
	"github.com/symphony-go/symphony/internal/workflow"
	"github.com/symphony-go/symphony/internal/workspace"
)

const version = "0.1.0"

func main() {
	var (
		portFlag    = flag.Int("port", 0, "HTTP server port (0 = disabled)")
		versionFlag = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()

	if *versionFlag {
		fmt.Printf("symphony-go %s\n", version)
		os.Exit(0)
	}

	logging.Setup()

	// Workflow path: positional arg or default
	workflowPath := "./WORKFLOW.md"
	if flag.NArg() > 0 {
		workflowPath = flag.Arg(0)
	}

	// Check file exists
	if _, err := os.Stat(workflowPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: workflow file not found: %s\n", workflowPath)
		os.Exit(1)
	}

	// Load and parse workflow
	wf, err := workflow.LoadWorkflow(workflowPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to load workflow: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.ParseConfig(wf.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to parse config: %v\n", err)
		os.Exit(1)
	}

	resolved, err := config.ResolveConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to resolve config: %v\n", err)
		os.Exit(1)
	}

	if err := config.ValidateDispatchConfig(resolved); err != nil {
		fmt.Fprintf(os.Stderr, "error: config validation failed: %v\n", err)
		os.Exit(1)
	}

	// Create components
	trackerClient, err := tracker.NewTrackerClient(&resolved.Tracker)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create tracker client: %v\n", err)
		os.Exit(1)
	}
	workspaceMgr := workspace.NewManager(resolved.Workspace.Root, &resolved.Hooks)

	cmuxMgr := cmux.New(&resolved.Cmux)
	if resolved.Cmux.Enabled {
		if err := cmuxMgr.Init(); err != nil {
			slog.Warn("cmux initialization failed, continuing without visibility", "error", err)
		}
	}
	defer cmuxMgr.Shutdown()

	launcher, err := agent.NewLauncher(resolved.Backend)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	orch := orchestrator.New(resolved, wf, trackerClient, launcher, workspaceMgr, cmuxMgr)

	// Start workflow watcher
	stopWatch, err := workflow.WatchWorkflow(workflowPath, func(newWf *workflow.WorkflowDefinition, newCfg *config.Config) {
		orch.ReloadCh() <- orchestrator.ReloadPayload{
			Config:   newCfg,
			Workflow: newWf,
		}
	})
	if err != nil {
		slog.Warn("failed to start workflow watcher", "error", err)
	} else {
		defer stopWatch()
	}

	// Determine HTTP server port
	httpPort := 0
	if resolved.Server.Port != nil {
		httpPort = *resolved.Server.Port
	}
	if *portFlag > 0 {
		httpPort = *portFlag // CLI overrides config
	}

	// Start HTTP server if port configured
	if httpPort != 0 {
		srv := server.New(httpPort, orch)
		if err := srv.Start(); err != nil {
			slog.Error("failed to start HTTP server", "error", err)
		} else {
			defer srv.Shutdown(context.Background())
			slog.Info("HTTP server started", "port", httpPort)
		}
	}

	// Signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	// Determine agent model/command for logging
	agentModel := resolved.Gemini.Model
	agentCommand := resolved.Gemini.Command
	if resolved.Backend == "claude" {
		agentModel = resolved.Claude.Model
		agentCommand = resolved.Claude.Command
	} else if resolved.Backend == "mini_agent" || resolved.Backend == "mini-agent" {
		agentModel = resolved.MiniAgent.Model
		agentCommand = resolved.MiniAgent.Command
	}

	slog.Info("symphony-go starting",
		"version", version,
		"backend", resolved.Backend,
		"tracker", resolved.Tracker.Kind,
		"project", resolved.Tracker.ProjectSlug,
		"agent_command", agentCommand,
		"agent_model", agentModel,
		"workspace_root", resolved.Workspace.Root,
		"poll_interval_ms", resolved.Polling.IntervalMs,
		"cmux_enabled", resolved.Cmux.Enabled,
	)

	// Run orchestrator (blocks until shutdown)
	if err := orch.Run(ctx); err != nil {
		slog.Error("orchestrator error", "error", err)
		os.Exit(1)
	}

	slog.Info("symphony-go stopped")
}
