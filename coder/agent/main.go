package main

import (
	"context"
	"fmt"
	"gigo-ws/coder/agent/agent"
	"gigo-ws/coder/agent/agent/reaper"
	"log"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/bwmarrin/snowflake"
	"github.com/gage-technologies/gigo-lib/buildinfo"
	"github.com/gage-technologies/gigo-lib/coder/agentsdk"
	"golang.org/x/xerrors"
	"gopkg.in/natefinch/lumberjack.v2"
)

// dumpHandler provides a custom SIGQUIT and SIGTRAP handler that dumps the
// stacktrace of all goroutines to stderr and a well-known file in the home
// directory. This is useful for debugging deadlock issues that may occur in
// production in workspaces, since the default Go runtime will only dump to
// stderr (which is often difficult/impossible to read in a workspace).
//
// SIGQUITs will still cause the program to exit (similarly to the default Go
// runtime behavior).
//
// A SIGQUIT handler will not be registered if GOTRACEBACK=crash.
//
// On Windows this immediately returns.
func dumpHandler(ctx context.Context) {
	if runtime.GOOS == "windows" {
		// free up the goroutine since it'll be permanently blocked anyways
		return
	}

	listenSignals := []os.Signal{syscall.SIGTRAP}
	if os.Getenv("GOTRACEBACK") != "crash" {
		listenSignals = append(listenSignals, syscall.SIGQUIT)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, listenSignals...)
	defer signal.Stop(sigs)

	for {
		sigStr := ""
		select {
		case <-ctx.Done():
			return
		case sig := <-sigs:
			switch sig {
			case syscall.SIGQUIT:
				sigStr = "SIGQUIT"
			case syscall.SIGTRAP:
				sigStr = "SIGTRAP"
			}
		}

		// Start with a 1MB buffer and keep doubling it until we can fit the
		// entire stacktrace, stopping early once we reach 64MB.
		buf := make([]byte, 1_000_000)
		stacklen := 0
		for {
			stacklen = runtime.Stack(buf, true)
			if stacklen < len(buf) {
				break
			}
			if 2*len(buf) > 64_000_000 {
				// Write a message to the end of the buffer saying that it was
				// truncated.
				const truncatedMsg = "\n\n\nstack trace truncated due to size\n"
				copy(buf[len(buf)-len(truncatedMsg):], truncatedMsg)
				break
			}
			buf = make([]byte, 2*len(buf))
		}

		_, _ = fmt.Fprintf(os.Stderr, "%s:\n%s\n", sigStr, buf[:stacklen])

		// Write to a well-known file.
		dir, err := os.UserHomeDir()
		if err != nil {
			dir = os.TempDir()
		}
		fpath := filepath.Join(dir, fmt.Sprintf("gigo-agent-%s.dump", time.Now().Format("2006-01-02T15:04:05.000Z")))
		_, _ = fmt.Fprintf(os.Stderr, "writing dump to %q\n", fpath)

		f, err := os.Create(fpath)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to open dump file: %v\n", err.Error())
			goto done
		}
		_, err = f.Write(buf[:stacklen])
		_ = f.Close()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to write dump file: %v\n", err.Error())
			goto done
		}

	done:
		if sigStr == "SIGQUIT" {
			//nolint:revive
			os.Exit(1)
		}
	}
}

func serveHandler(ctx context.Context, logger slog.Logger, handler http.Handler, addr, name string) (closeFunc func()) {
	logger.Debug(ctx, "http server listening", slog.F("addr", addr), slog.F("name", name))

	// ReadHeaderTimeout is purposefully not enabled. It caused some issues with
	// websockets over the dev tunnel.
	// See: https://github.com/coder/coder/pull/3730
	//nolint:gosec
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !xerrors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, "http server listen", slog.F("name", name), slog.Error(err))
		}
	}()

	return func() {
		_ = srv.Close()
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	go dumpHandler(ctx)

	// TODO: update this to gigo based env var after provisioner dev
	coreUrlRaw := os.Getenv("GIGO_AGENT_URL")
	if len(coreUrlRaw) == 0 {
		log.Fatal("GIGO_AGENT_URL must be set")
	}
	coreUrl, err := url.Parse(coreUrlRaw)
	if err != nil {
		log.Fatalf("failed to parse GIGO_AGENT_URL: %v", err)
	}

	// retrieve agent id
	agentIdString := os.Getenv("GIGO_AGENT_ID")
	if len(agentIdString) == 0 {
		log.Fatal("GIGO_AGENT_ID must be set")
	}

	// format agent id string to int
	agentId, err := strconv.ParseInt(agentIdString, 10, 64)
	if err != nil {
		log.Fatalf("failed to parse GIGO_AGENT_ID: %v", err)
	}

	// retrieve auth fields for api client from environment
	token := os.Getenv("GIGO_AGENT_TOKEN")
	if len(token) == 0 {
		log.Fatal("GIGO_AGENT_TOKEN must be set")
	}
	workspaceIdString := os.Getenv("GIGO_WORKSPACE_ID")
	if len(workspaceIdString) == 0 {
		log.Fatal("GIGO_WORKSPACE_ID must be set")
	}

	// format workspace id string to int
	workspaceId, err := strconv.ParseInt(workspaceIdString, 10, 64)
	if err != nil {
		log.Fatalf("failed to parse GIGO_WORKSPACE_ID: %v", err)
	}

	// always set this node to 1023 since we reserve
	// that node id for agents this way any id generated
	// on an agent can always be tracked back to the agent
	// codebase
	snowflakeNode, err := snowflake.NewNode(1023)
	if err != nil {
		log.Fatalf("failed to create snowflake node: %v", err)
	}

	// create logger for agent
	logWriter := &lumberjack.Logger{
		Filename: filepath.Join(os.TempDir(), "gigo-agent.log"),
		MaxSize:  5, // MB
	}
	defer logWriter.Close()
	logger := slog.Make(sloghuman.Sink(os.Stdout), sloghuman.Sink(logWriter)).Leveled(slog.LevelDebug)

	isLinux := runtime.GOOS == "linux"

	// Spawn a reaper so that we don't accumulate a ton
	// of zombie processes.
	if reaper.IsInitProcess() && isLinux {
		logger.Info(ctx, "spawning reaper process")
		// Do not start a reaper on the child process. It's important
		// to do this else we fork bomb ourselves.
		args := append(os.Args, "--no-reap")
		err := reaper.ForkReap(reaper.WithExecArgs(args...))
		if err != nil {
			logger.Error(ctx, "failed to reap", slog.Error(err))
			return
		}

		logger.Info(ctx, "reaper process exiting")
		return
	}

	version := buildinfo.Version()
	logger.Info(ctx, "starting agent",
		slog.F("url", coreUrl),
		slog.F("version", version.Version),
	)
	client := agentsdk.New(coreUrl)
	client.Logger = logger
	// Set a reasonable timeout so requests can't hang forever!
	client.HTTPClient.Timeout = 10 * time.Second

	// set client auth
	client.SetSessionAuth(workspaceId, token)

	// Enable pprof handler
	// This prevents the pprof import from being accidentally deleted.
	_ = pprof.Handler
	pprofSrvClose := serveHandler(ctx, logger, nil, "127.0.0.1:6060", "pprof")
	defer pprofSrvClose()

	executablePath, err := os.Executable()
	if err != nil {
		log.Fatalf("getting os executable: %v", err)
	}
	err = os.Setenv("PATH", fmt.Sprintf("%s%c%s", os.Getenv("PATH"), filepath.ListSeparator, filepath.Dir(executablePath)))
	if err != nil {
		log.Fatalf("add executable to $PATH: %v", err)
	}

	closer := agent.New(agent.Options{
		ID:                   agentId,
		Client:               client,
		AccessUrl:            coreUrl,
		Logger:               logger,
		EnvironmentVariables: make(map[string]string),
		SnowflakeNode:        snowflakeNode,
	})
	<-ctx.Done()
	_ = closer.Close()
}
