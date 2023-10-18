package agent

import (
	"cdr.dev/slog"
	"context"
	"github.com/coder/retry"
	"github.com/gage-technologies/gigo-lib/db/models"
	"golang.org/x/xerrors"
	"time"
)

// reportAgentStateLoop reports the current agent state once.
// Only the latest state is reported, intermediate states may be
// lost if the agent can't communicate with the API.
func (a *agent) reportAgentStateLoop(ctx context.Context) {
	var lastReported models.WorkspaceAgentState
	for {
		select {
		case <-a.agentStateUpdate:
		case <-ctx.Done():
			return
		}

		for r := retry.New(time.Second, 15*time.Second); r.Wait(ctx); {
			a.agentStateLock.Lock()
			state := a.agentState
			a.agentStateLock.Unlock()

			if state == lastReported {
				break
			}

			a.logger.Debug(ctx, "post lifecycle state", slog.F("state", state))

			err := a.client.PostWorkspaceAgentState(ctx, state)
			if err == nil {
				lastReported = state
				break
			}
			if xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
				return
			}
			// If we fail to report the state we probably shouldn't exit, log only.
			a.logger.Error(ctx, "post state", slog.Error(err))
		}
	}
}

func (a *agent) setAgentState(ctx context.Context, state models.WorkspaceAgentState) {
	a.agentStateLock.Lock()
	defer a.agentStateLock.Unlock()

	a.logger.Debug(ctx, "set lifecycle state", slog.F("state", state), slog.F("previous", a.agentState))

	a.agentState = state
	select {
	case a.agentStateUpdate <- struct{}{}:
	default:
	}
}
