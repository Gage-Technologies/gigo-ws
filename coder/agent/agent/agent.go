package agent

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"cdr.dev/slog"
	"github.com/bwmarrin/snowflake"
	"github.com/coder/retry"
	"github.com/gage-technologies/gigo-lib/buildinfo"
	"github.com/gage-technologies/gigo-lib/coder/agentsdk"
	"github.com/gage-technologies/gigo-lib/db/models"
	"github.com/gage-technologies/gigo-lib/zitimesh"
	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	"github.com/spf13/afero"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
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
	AccessUrl              *url.URL
	ReconnectingPTYTimeout time.Duration
	EnvironmentVariables   map[string]string
	Logger                 slog.Logger
	SnowflakeNode          *snowflake.Node
}

type agent struct {
	id            int64
	logger        slog.Logger
	client        Client
	filesystem    afero.Fs
	tempDir       string
	snowflakeNode *snowflake.Node
	accessUrl     *url.URL

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

	zitiAgent     *zitimesh.Agent
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
		accessUrl:              options.AccessUrl,
		filesystem:             options.Filesystem,
		tempDir:                options.TempDir,
		agentStateUpdate:       make(chan struct{}, 1),
		snowflakeNode:          options.SnowflakeNode,
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

	a.logger.Debug(ctx, "creating ziti agent")
	err = a.createZitiAgent(ctx)
	if err != nil {
		a.logger.Debug(ctx, "failed creating ziti agent", slog.Error(err))
		return xerrors.Errorf("create ziti agent: %w", err)
	}

	durationChangeChan := make(chan time.Duration)
	setStatInterval := func(d time.Duration) {
		durationChangeChan <- d
	}
	go a.watchNetworkStats(ctx, durationChangeChan)

	// Report statistics from the ziti network.
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

	return nil
}

func (a *agent) watchNetworkStats(ctx context.Context, durationChangeChan <-chan time.Duration) {
	ticker := time.NewTicker(time.Minute * 10)
	for {
		select {
		case <-ctx.Done():
			return
		case d := <-durationChangeChan:
			ticker.Reset(d)
		case <-ticker.C:
			if a.zitiAgent == nil {
				continue
			}
			// retrieve the latest network stats
			stats := a.zitiAgent.GetNetworkStats()
			// clear the net stats
			a.zitiAgent.ClearStats()
			// convert the stats into agent format and write to the channel
			a.connStatsChan <- convertAgentStats(stats)
		}
	}
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

func (a *agent) createZitiAgent(ctx context.Context) error {
	// load the metadata to get the ziti token
	rawMetadata := a.metadata.Load()
	if rawMetadata == nil {
		return xerrors.Errorf("no metadata was provided")
	}
	metadata, valid := rawMetadata.(agentsdk.WorkspaceAgentMetadata)
	if !valid {
		return xerrors.Errorf("metadata is the wrong type: %T", metadata)
	}

	var err error
	a.zitiAgent, err = zitimesh.NewAgent(ctx, metadata.ZitiID, metadata.ZitiToken, a.logger)
	if err != nil {
		return xerrors.Errorf("failed to create ziti agent: %w", err)
	}

	return nil
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
	if a.zitiAgent != nil {
		a.zitiAgent.Close()
	}
	_ = a.sshServer.Close()
	a.connCloseWait.Wait()
	return nil
}
