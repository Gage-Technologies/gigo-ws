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

// initConnectionServer
//
// Launches a server with a /ping endpoint used to establish
// ziti tunneling from the server before the rest of the agent
// is online.
// This makes the user experience better because by the time
// the agent is fully online the tunneling has occurred.
func (a *agent) initConnectionServer(ctx context.Context) {
	// Define the handler for the /ping endpoint
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		a.logger.Info(ctx, "received init connection ping")
		fmt.Fprintln(w, "pong")
	})

	apiServer, err := server2.NewHttpApi(server2.HttpApiParams{NodeID: a.id, Snowflake: a.snowflakeNode, Port: agentsdk.ZitiInitConnPort, Host: "localhost", Logger: a.logger, Secret: a.client.SessionAuth().Token})
	if err != nil {
		a.logger.Error(ctx, "failed to create api server", slog.Error(err))
		return
	}

	//// Define the server and its properties
	//server := &http.Server{Addr: fmt.Sprintf("localhost:%d", agentsdk.ZitiInitConnPort)} // You can choose an appropriate port

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
		agentsdk.ZitiInitConnPort: {},
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
