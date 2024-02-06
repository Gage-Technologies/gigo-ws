package agent

import (
	"context"
	"fmt"
	server2 "gigo-ws/coder/agent/agent/server"
	"net"
	"net/http"

	"cdr.dev/slog"
	"github.com/gage-technologies/gigo-lib/coder/agentsdk"
)

// agentControlServer
//
// Launches a server with agent controls that can be accessed via a ws
// to enable code execution and linting for Bytes.
func (a *agent) agentControlServer(ctx context.Context) {
	apiServer, err := server2.NewHttpApi(server2.HttpApiParams{
		NodeID:    a.id,
		Snowflake: a.snowflakeNode,
		Port:      agentsdk.ZitiAgentServerPort,
		Host:      "localhost",
		Logger:    a.logger,
		Secret:    a.client.SessionAuth().Token,
	})
	if err != nil {
		a.logger.Error(ctx, "failed to create api server", slog.Error(err))
		return
	}

	// Start the server in a goroutine so it doesn't block
	go func() {
		if err := apiServer.Start(ctx); err != nil && err != http.ErrServerClosed {
			a.logger.Error(ctx, "init connection server failed", slog.Error(err))
		}
	}()

	// Listen for context cancellation and shutdown the server when it's cancelled
	go func() {
		<-ctx.Done()
		if err := apiServer.Shutdown(context.Background()); err != nil {
			a.logger.Error(ctx, "init connection server shutdown error", slog.Error(err))
		}
	}()
}

// reserveUnusedGigoPorts
//
// Reserves ports for Gigo services that are not yet implemented
func (a *agent) reserveUnusedGigoPorts(ctx context.Context) {
	// ports that are in use and should be skipped
	used := map[int]struct{}{
		agentsdk.ZitiAgentServerPort: {},
		agentsdk.ZitiAgentLspWsPort:  {},
	}

	// create listeners on all of the reserved ports as
	// long as they are not in `used`
	for port, _ := range used {
		// skip if the port is already in use
		if _, ok := used[port]; ok {
			continue
		}

		// open a listener to reserve the port
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			a.logger.Error(ctx, "failed to reserve port", slog.F("port", port), slog.Error(err))
			continue
		}
		defer listener.Close()
		a.logger.Info(ctx, "reserved port", slog.F("port", port))
	}
}
