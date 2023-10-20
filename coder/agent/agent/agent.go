package agent

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"cdr.dev/slog"
	"github.com/bwmarrin/snowflake"
	"github.com/coder/retry"
	"github.com/gage-technologies/gigo-lib/buildinfo"
	"github.com/gage-technologies/gigo-lib/coder/agentsdk"
	"github.com/gage-technologies/gigo-lib/coder/tailnet"
	"github.com/gage-technologies/gigo-lib/db/models"
	"github.com/gage-technologies/gigo-lib/logging"
	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	"github.com/spf13/afero"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
	"tailscale.com/net/speedtest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/netlogtype"
)

const (
	ProtocolReconnectingPTY = "reconnecting-pty"
	ProtocolSSH             = "ssh"
	ProtocolDial            = "dial"

	// MagicSessionErrorCode indicates that something went wrong with the session, rather than the
	// command just returning a nonzero exit code, and is chosen as an arbitrary, high number
	// unlikely to shadow other exit codes, which are typically 1, 2, 3, etc.
	MagicSessionErrorCode = 229
)

type Client interface {
	InitializeWorkspaceAgent(ctx context.Context, isVnc bool) (agentsdk.WorkspaceAgentMetadata, error)
	ListenWorkspaceAgent(ctx context.Context) (net.Conn, error)
	PostWorkspaceAgentState(ctx context.Context, state models.WorkspaceAgentState) error
	PostWorkspaceAgentVersion(ctx context.Context, version string) error
	PostAgentPorts(ctx context.Context, req *agentsdk.AgentPorts) error
	AgentReportStats(ctx context.Context, log slog.Logger, statsChan <-chan *agentsdk.AgentStats, setInterval func(time.Duration)) (io.Closer, error)
	WorkspaceInitializationStepCompleted(ctx context.Context, state models.WorkspaceInitState) error
	WorkspaceInitializationFailure(ctx context.Context, req agentsdk.PostWorkspaceInitFailure) error
	WorkspaceGetExtension(ctx context.Context, extPath string) error
	WorkspaceGetCtExtension(ctx context.Context, extPath string) error
	WorkspaceGetThemeExtension(ctx context.Context, extPath string) error
	WorkspaceGetHolidayThemeExtension(ctx context.Context, extPath string, holiday int) error
	WorkspaceGetOpenVsxExtension(ctx context.Context, extId, version, vscVersion, extPath string) error
	SessionAuth() agentsdk.AgentAuth
}

type Options struct {
	ID                     int64
	Filesystem             afero.Fs
	TempDir                string
	Client                 Client
	ReconnectingPTYTimeout time.Duration
	EnvironmentVariables   map[string]string
	Logger                 slog.Logger
	TailnetLogger          logging.Logger
	SnowflakeNode          *snowflake.Node
}

type agent struct {
	id     int64
	logger slog.Logger
	// we use a different logger for tailnet to keep consistent
	// with the backend systems that use a custom interface over
	// a logrus based logger
	tailnetLogger logging.Logger
	client        Client
	filesystem    afero.Fs
	tempDir       string
	snowflakeNode *snowflake.Node

	reconnectingPTYs       sync.Map
	reconnectingPTYTimeout time.Duration

	connCloseWait sync.WaitGroup
	closeCancel   context.CancelFunc
	closeMutex    sync.Mutex
	closed        chan struct{}

	envVars map[string]string
	// metadata is atomic because values can change after reconnection.
	metadata  atomic.Value
	sshServer *ssh.Server

	agentStateUpdate chan struct{}
	agentStateLock   sync.Mutex // Protects following.
	agentState       models.WorkspaceAgentState

	network       *tailnet.Conn
	connStatsChan chan *agentsdk.AgentStats
}

func New(options Options) io.Closer {
	if options.ReconnectingPTYTimeout == 0 {
		options.ReconnectingPTYTimeout = 5 * time.Minute
	}
	if options.Filesystem == nil {
		options.Filesystem = afero.NewOsFs()
	}
	if options.TempDir == "" {
		options.TempDir = os.TempDir()
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	a := &agent{
		id:                     options.ID,
		reconnectingPTYTimeout: options.ReconnectingPTYTimeout,
		logger:                 options.Logger,
		closeCancel:            cancelFunc,
		closed:                 make(chan struct{}),
		envVars:                options.EnvironmentVariables,
		client:                 options.Client,
		filesystem:             options.Filesystem,
		tempDir:                options.TempDir,
		agentStateUpdate:       make(chan struct{}, 1),
		snowflakeNode:          options.SnowflakeNode,
		tailnetLogger:          options.TailnetLogger,
		connStatsChan:          make(chan *agentsdk.AgentStats, 1),
	}
	a.init(ctx)
	return a
}

// runLoop attempts to start the agent in a retry loop.
// Coder may be offline temporarily, a connection issue
// may be happening, but regardless after the intermittent
// failure, you'll want the agent to reconnect.
func (a *agent) runLoop(ctx context.Context) {
	go a.reportAgentStateLoop(ctx)
	go a.portTrackerRoutine(ctx)

	for retrier := retry.New(100*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
		a.logger.Info(ctx, "running loop")
		err := a.run(ctx)
		// Cancel after the run is complete to clean up any leaked resources!
		if err == nil {
			continue
		}
		if errors.Is(err, context.Canceled) {
			return
		}
		if a.isClosed() {
			return
		}
		if errors.Is(err, io.EOF) {
			a.logger.Info(ctx, "likely disconnected from coder", slog.Error(err))
			continue
		}
		a.logger.Warn(ctx, "run exited with error", slog.Error(err))
	}
}

func (a *agent) run(ctx context.Context) error {
	err := a.client.PostWorkspaceAgentVersion(ctx, buildinfo.Version().Version)
	if err != nil {
		return xerrors.Errorf("update workspace agent version: %w", err)
	}

	// detect if the vnc file exists to determine if we are using vnc or not
	isVnc := false
	_, err = os.Stat("/gigo/vnc")
	if err == nil {
		isVnc = true
	}

	metadata, err := a.client.InitializeWorkspaceAgent(ctx, isVnc)
	if err != nil {
		// mark init failure since this is part of the remote
		// initialization state on workspace initialization
		a.reportStateFailure(ctx, models.WorkspaceInitRemoteInitialization, err, nil)
		return xerrors.Errorf("fetch metadata: %w", err)
	}

	if metadata.WorkspaceState == models.WorkspaceFailed {
		ctx.Done()
		return nil
	}
	a.logger.Info(ctx, "fetched metadata")
	oldMetadata := a.metadata.Swap(metadata)

	// The startup script should only execute on the first run!
	if oldMetadata == nil {
		// mark agent state as starting
		a.setAgentState(ctx, models.WorkspaceAgentStateStarting)

		// create an error channel to track the execution
		// of the bootstrap
		bootstrapDone := make(chan error, 1)
		bootstrapStart := time.Now()

		// launch bootstrap logic
		go func() {
			defer close(bootstrapDone)
			// bootstrap workspace
			bootstrapDone <- a.runBootstrap(ctx)
		}()

		// launch state update for bootstrap success
		go func() {
			var timeout <-chan time.Time

			// we hardcode this to 7 minutes because the user is given
			// a total of 10 minutes to establish a connection to the
			// workspace in some form or another
			t := time.NewTimer(7 * time.Minute)
			defer t.Stop()
			timeout = t.C

			// create an error so that we can
			// cache the error temporarily while
			// we calculate the runtime if we fail
			var err error

			// wait for success, failure or timeout
			select {
			// handle success or failure
			case err = <-bootstrapDone:

			// handle timeout
			case <-timeout:
				a.logger.Warn(ctx, "startup script timed out")
				a.setAgentState(ctx, models.WorkspaceAgentStateTimeout)
				err = <-bootstrapDone // The script can still complete after a timeout.
			}

			// TODO: track back where the context could be cancelled

			// exit if we cancelled the bootstrap
			if errors.Is(err, context.Canceled) {
				return
			}

			execTime := time.Since(bootstrapStart)

			if err != nil {
				a.logger.Warn(ctx, "workspace bootstrap failed", slog.F("execution_time", execTime), slog.Error(err))
				// update agent state to mark failure
				a.setAgentState(ctx, models.WorkspaceAgentStateFailed)
				return
			}

			a.logger.Info(ctx, "workspace bootstrap completed", slog.F("execution_time", execTime))

			// create channel to track closure of code-server
			// or to wait forever if the user did not configure
			// an editor
			codeServerLive := make(chan bool)
			defer close(codeServerLive)

			// launch code-server if requested by user we are ignoring the atomic nature of the
			// agents config since this should not change regardless of how often if is initialized
			if metadata.GigoConfig.VSCode.Enabled {
				a.logger.Info(ctx, "launching code-server or waiting forever", slog.F("execution_time", execTime))

				// launch code server in a separate thread
				go func() {
					defer func() {
						// write false to indicate that code server failed
						codeServerLive <- false
					}()
					res, err := a.runCodeServer(ctx)
					if err != nil || (res != nil && res.ExitCode != 0) {
						a.logger.Error(ctx, "cs exit 1", slog.F("res", res), slog.Error(err))
						a.reportStateFailure(ctx, models.WorkspaceInitVSCodeLaunch, err, res)
						// update agent state to mark failure
						a.setAgentState(ctx, models.WorkspaceAgentStateFailed)
					}
					a.logger.Info(ctx, "code-server exited")
				}()

				// launch health wait in a separate thread
				go func() {
					a.waitHealthyCodeServer(ctx, codeServerLive, metadata)
				}()

				// mark code-server launch as completed
				a.reportStateCompleted(ctx, models.WorkspaceInitVSCodeLaunch)

				// wait for code-server failure or healthy code-server
				codeServerAlive := <-codeServerLive

				// mark failure and return if code server died
				if !codeServerAlive {
					a.logger.Error(ctx, "cs exit 2", slog.Error(err))
					a.reportStateFailure(ctx, models.WorkspaceInitVSCodeLaunch, err, nil)
					// update agent state to mark failure
					a.setAgentState(ctx, models.WorkspaceAgentStateFailed)
					// TODO: investigate the best exit strategy
					a.closeCancel()
					return
				}
			}

			// mark initialization as completed
			a.reportStateCompleted(ctx, models.WorkspaceInitCompleted)
			// update agent state
			a.setAgentState(ctx, models.WorkspaceAgentStateRunning)

			// wait for closure - or forever
			<-codeServerLive
		}()
	}

	// TODO: really think about if we want to have an app type and system
	// This automatically closes when the context ends!
	// appReporterCtx, appReporterCtxCancel := context.WithCancel(ctx)
	// defer appReporterCtxCancel()
	// go NewWorkspaceAppHealthReporter(
	// 	a.logger, metadata.Apps, a.client.PostWorkspaceAgentAppHealth)(appReporterCtx)

	a.logger.Debug(ctx, "running tailnet with derpmap", slog.F("derpmap", metadata.DERPMap))

	a.closeMutex.Lock()
	network := a.network
	a.closeMutex.Unlock()
	if network == nil {
		a.logger.Debug(ctx, "creating tailnet")
		network, err = a.createTailnet(ctx, metadata.DERPMap)
		if err != nil {
			return xerrors.Errorf("create tailnet: %w", err)
		}
		a.closeMutex.Lock()
		// Re-check if agent was closed while initializing the network.
		closed := a.isClosed()
		if !closed {
			a.network = network
		}
		a.closeMutex.Unlock()
		if closed {
			_ = network.Close()
			return xerrors.New("agent is closed")
		}

		setStatInterval := func(d time.Duration) {
			network.SetConnStatsCallback(d, 2048,
				func(_, _ time.Time, virtual, _ map[netlogtype.Connection]netlogtype.Counts) {
					select {
					case a.connStatsChan <- convertAgentStats(virtual):
					default:
						a.logger.Warn(ctx, "network stat dropped")
					}
				},
			)
		}

		// Report statistics from the created network.
		cl, err := a.client.AgentReportStats(ctx, a.logger, a.connStatsChan, setStatInterval)
		if err != nil {
			a.logger.Error(ctx, "report stats", slog.Error(err))
		} else {
			if err = a.trackConnGoroutine(func() {
				// This is OK because the agent never re-creates the tailnet
				// and the only shutdown indicator is agent.Close().
				<-a.closed
				_ = cl.Close()
			}); err != nil {
				a.logger.Debug(ctx, "report stats goroutine", slog.Error(err))
				_ = cl.Close()
			}
		}
	} else {
		// Update the DERP map!
		network.SetDERPMap(metadata.DERPMap)
	}

	a.logger.Debug(ctx, "running coordinator")
	err = a.runCoordinator(ctx, network)
	if err != nil {
		a.logger.Debug(ctx, "coordinator exited", slog.Error(err))
		return xerrors.Errorf("run coordinator: %w", err)
	}
	return nil
}

func (a *agent) trackConnGoroutine(fn func()) error {
	a.closeMutex.Lock()
	defer a.closeMutex.Unlock()
	if a.isClosed() {
		return xerrors.New("track conn goroutine: agent is closed")
	}
	a.connCloseWait.Add(1)
	go func() {
		defer a.connCloseWait.Done()
		fn()
	}()
	return nil
}

func (a *agent) createTailnet(ctx context.Context, derpMap *tailcfg.DERPMap) (_ *tailnet.Conn, err error) {
	a.tailnetLogger.Infof("creating connnection with node id: %d", a.id)

	// TODO: evaluate if this needs to be set back to the agent id
	connectionID := a.snowflakeNode.Generate().Int64()
	network, err := tailnet.NewConn(tailnet.ConnTypeAgent, &tailnet.Options{
		NodeID:    connectionID,
		Addresses: []netip.Prefix{netip.PrefixFrom(agentsdk.TailnetIP, 128)},
		DERPMap:   derpMap,
	}, a.tailnetLogger)
	if err != nil {
		return nil, xerrors.Errorf("create tailnet: %w", err)
	}
	defer func() {
		if err != nil {
			network.Close()
		}
	}()

	sshListener, err := network.Listen("tcp", ":"+strconv.Itoa(agentsdk.TailnetSSHPort))
	if err != nil {
		return nil, xerrors.Errorf("listen on the ssh port: %w", err)
	}
	defer func() {
		if err != nil {
			_ = sshListener.Close()
		}
	}()
	if err = a.trackConnGoroutine(func() {
		for {
			conn, err := sshListener.Accept()
			if err != nil {
				return
			}
			closed := make(chan struct{})
			_ = a.trackConnGoroutine(func() {
				select {
				case <-network.Closed():
				case <-closed:
				}
				_ = conn.Close()
			})
			_ = a.trackConnGoroutine(func() {
				defer close(closed)
				a.sshServer.HandleConn(conn)
			})
		}
	}); err != nil {
		return nil, err
	}

	reconnectingPTYListener, err := network.Listen("tcp", ":"+strconv.Itoa(agentsdk.TailnetReconnectingPTYPort))
	if err != nil {
		return nil, xerrors.Errorf("listen for reconnecting pty: %w", err)
	}
	defer func() {
		if err != nil {
			_ = reconnectingPTYListener.Close()
		}
	}()
	if err = a.trackConnGoroutine(func() {
		logger := a.logger.Named("reconnecting-pty")

		for {
			conn, err := reconnectingPTYListener.Accept()
			if err != nil {
				logger.Debug(ctx, "accept pty failed", slog.Error(err))
				return
			}
			// This cannot use a JSON decoder, since that can
			// buffer additional data that is required for the PTY.
			rawLen := make([]byte, 2)
			_, err = conn.Read(rawLen)
			if err != nil {
				continue
			}
			length := binary.LittleEndian.Uint16(rawLen)
			data := make([]byte, length)
			_, err = conn.Read(data)
			if err != nil {
				continue
			}
			var msg agentsdk.ReconnectingPTYInit
			err = json.Unmarshal(data, &msg)
			if err != nil {
				continue
			}
			go func() {
				_ = a.handleReconnectingPTY(ctx, logger, msg, conn)
			}()
		}
	}); err != nil {
		return nil, err
	}

	speedtestListener, err := network.Listen("tcp", ":"+strconv.Itoa(agentsdk.TailnetSpeedtestPort))
	if err != nil {
		return nil, xerrors.Errorf("listen for speedtest: %w", err)
	}
	defer func() {
		if err != nil {
			_ = speedtestListener.Close()
		}
	}()
	if err = a.trackConnGoroutine(func() {
		for {
			conn, err := speedtestListener.Accept()
			if err != nil {
				a.logger.Debug(ctx, "speedtest listener failed", slog.Error(err))
				return
			}
			if err = a.trackConnGoroutine(func() {
				_ = speedtest.ServeConn(conn)
			}); err != nil {
				a.logger.Debug(ctx, "speedtest listener failed", slog.Error(err))
				_ = conn.Close()
				return
			}
		}
	}); err != nil {
		return nil, err
	}

	return network, nil
}

// runCoordinator runs a coordinator and returns whether a reconnect
// should occur.
func (a *agent) runCoordinator(ctx context.Context, network *tailnet.Conn) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	coordinatorConn, err := a.client.ListenWorkspaceAgent(ctx)
	if err != nil {
		return err
	}
	defer coordinatorConn.Close()
	a.logger.Info(ctx, "connected to coordination server")
	errChan := network.ConnectToCoordinator(coordinatorConn)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

func (a *agent) init(ctx context.Context) {
	a.logger.Info(ctx, "generating host key")
	// Clients' should ignore the host key when connecting.
	// The agent needs to authenticate with coderd to SSH,
	// so SSH authentication doesn't improve security.
	randomHostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	randomSigner, err := gossh.NewSignerFromKey(randomHostKey)
	if err != nil {
		panic(err)
	}

	sshLogger := a.logger.Named("ssh-server")
	forwardHandler := &ssh.ForwardedTCPHandler{}
	unixForwardHandler := &forwardedUnixHandler{log: a.logger}

	a.sshServer = &ssh.Server{
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip":                   ssh.DirectTCPIPHandler,
			"direct-streamlocal@openssh.com": directStreamLocalHandler,
			"session":                        ssh.DefaultSessionHandler,
		},
		ConnectionFailedCallback: func(conn net.Conn, err error) {
			sshLogger.Info(ctx, "ssh connection ended", slog.Error(err))
		},
		Handler: func(session ssh.Session) {
			err := a.handleSSHSession(session)
			var exitError *exec.ExitError
			if xerrors.As(err, &exitError) {
				a.logger.Debug(ctx, "ssh session returned", slog.Error(exitError))
				_ = session.Exit(exitError.ExitCode())
				return
			}
			if err != nil {
				a.logger.Warn(ctx, "ssh session failed", slog.Error(err))
				// This exit code is designed to be unlikely to be confused for a legit exit code
				// from the process.
				_ = session.Exit(MagicSessionErrorCode)
				return
			}
		},
		HostSigners: []ssh.Signer{randomSigner},
		LocalPortForwardingCallback: func(ctx ssh.Context, destinationHost string, destinationPort uint32) bool {
			// Allow local port forwarding all!
			sshLogger.Debug(ctx, "local port forward",
				slog.F("destination-host", destinationHost),
				slog.F("destination-port", destinationPort))
			return true
		},
		PtyCallback: func(ctx ssh.Context, pty ssh.Pty) bool {
			return true
		},
		ReversePortForwardingCallback: func(ctx ssh.Context, bindHost string, bindPort uint32) bool {
			// Allow reverse port forwarding all!
			sshLogger.Debug(ctx, "local port forward",
				slog.F("bind-host", bindHost),
				slog.F("bind-port", bindPort))
			return true
		},
		RequestHandlers: map[string]ssh.RequestHandler{
			"tcpip-forward":                          forwardHandler.HandleSSHRequest,
			"cancel-tcpip-forward":                   forwardHandler.HandleSSHRequest,
			"streamlocal-forward@openssh.com":        unixForwardHandler.HandleSSHRequest,
			"cancel-streamlocal-forward@openssh.com": unixForwardHandler.HandleSSHRequest,
		},
		ServerConfigCallback: func(ctx ssh.Context) *gossh.ServerConfig {
			return &gossh.ServerConfig{
				NoClientAuth: true,
			}
		},
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": func(session ssh.Session) {
				ctx := session.Context()

				// Typically sftp sessions don't request a TTY, but if they do,
				// we must ensure the gliderlabs/ssh CRLF emulation is disabled.
				// Otherwise sftp will be broken. This can happen if a user sets
				// `RequestTTY force` in their SSH config.
				session.DisablePTYEmulation()

				var opts []sftp.ServerOption
				// Change current working directory to the users home
				// directory so that SFTP connections land there.
				homedir, err := userHomeDir()
				if err != nil {
					sshLogger.Warn(ctx, "get sftp working directory failed, unable to get home dir", slog.Error(err))
				} else {
					opts = append(opts, sftp.WithServerWorkingDirectory(homedir))
				}

				server, err := sftp.NewServer(session, opts...)
				if err != nil {
					sshLogger.Debug(ctx, "initialize sftp server", slog.Error(err))
					return
				}
				defer server.Close()

				err = server.Serve()
				if errors.Is(err, io.EOF) {
					// Unless we call `session.Exit(0)` here, the client won't
					// receive `exit-status` because `(*sftp.Server).Close()`
					// calls `Close()` on the underlying connection (session),
					// which actually calls `channel.Close()` because it isn't
					// wrapped. This causes sftp clients to receive a non-zero
					// exit code. Typically sftp clients don't echo this exit
					// code but `scp` on macOS does (when using the default
					// SFTP backend).
					_ = session.Exit(0)
					return
				}
				sshLogger.Warn(ctx, "sftp server closed with error", slog.Error(err))
				_ = session.Exit(1)
			},
		},
	}

	go a.runLoop(ctx)
}

// isClosed returns whether the API is closed or not.
func (a *agent) isClosed() bool {
	select {
	case <-a.closed:
		return true
	default:
		return false
	}
}

func (a *agent) Close() error {
	// TODO: make sure we are closing everything
	// we may have added a routine that isn't
	// guaranteed to close on this logic
	a.closeMutex.Lock()
	defer a.closeMutex.Unlock()
	if a.isClosed() {
		return nil
	}
	close(a.closed)
	a.closeCancel()
	if a.network != nil {
		_ = a.network.Close()
	}
	_ = a.sshServer.Close()
	a.connCloseWait.Wait()
	return nil
}
