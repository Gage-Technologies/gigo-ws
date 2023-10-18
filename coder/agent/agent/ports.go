package agent

import (
	"cdr.dev/slog"
	"context"
	"fmt"
	"github.com/coder/retry"
	"github.com/gage-technologies/gigo-lib/coder/agentsdk"
	"github.com/gage-technologies/gigo-lib/utils"
	"sort"
	"strings"
	"time"
)

// ActivePortState
//
//	Tracks the current state of active ports on the
//	deployed machine. This struct provides convenience
//	functions to determine if the state has changed and
//	what the state changes are.
type ActivePortState map[uint16]*agentsdk.ListeningPort

// Hash
//
//	Hashes the ActivePortState in a consistent manner.
func (a ActivePortState) Hash() (string, error) {
	// retrieve keys and sort them so we can hash the map in
	// a consistent order
	keys := make([]uint16, len(a))
	ki := 0
	for k := range a {
		keys[ki] = k
		ki++
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	// create string from state
	stateStrings := make([]string, len(keys))
	for i, k := range keys {
		stateStrings[i] = a[k].String()

	}
	stateString := strings.Join(stateStrings, "-")

	// hash state string and return
	h, err := utils.HashData([]byte(stateString))
	if err != nil {
		return "", fmt.Errorf("error hashing state: %v", err)
	}

	return h, nil
}

// Equals
//
//	Whether the passed ActivePortState is equal to the
//	current ActivePortState.
func (a ActivePortState) Equals(b ActivePortState) (bool, error) {
	// easy check for length change
	if len(a) != len(b) {
		return false, nil
	}

	// calculate hashes
	ah, err := a.Hash()
	if err != nil {
		return false, fmt.Errorf("failed to hash a: %v", err)
	}
	bh, err := b.Hash()
	if err != nil {
		return false, fmt.Errorf("failed to hash b: %v", err)
	}

	// compare hashes
	return ah == bh, nil
}

// portTrackerRoutine
//
//	Watches for port changes and sends updates
//	the GIGO system with the current active ports.
func (a *agent) portTrackerRoutine(ctx context.Context) {
	// create map to track the state of ports
	state := make(ActivePortState)

	// create a ticker to trigger updates every second
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// loop forever tracking port changes
	for {
		// wait for one second or the context is cancelled
		select {
		case <-ctx.Done():
			// we are done if the context is cancelled
			return
		case <-ticker.C:
		}

		// retrieve the current active ports
		activePorts, err := getListeningPorts()
		if err != nil {
			a.logger.Error(ctx, "error retrieving active ports", slog.Error(err))
			continue
		}

		// skip if there is no change to the active ports
		equal, err := state.Equals(activePorts)
		if err != nil {
			a.logger.Error(ctx, "error comparing port states", slog.Error(err))
			continue
		}
		if equal {
			continue
		}

		// update port state
		state = activePorts

		// format port state to sdk format
		req := &agentsdk.AgentPorts{
			Ports: make([]agentsdk.ListeningPort, len(state)),
		}
		si := 0
		for _, v := range state {
			req.Ports[si] = *v
			si++
		}

		// execute update call to backend
		for retrier := retry.New(100*time.Millisecond, time.Second); retrier.Wait(ctx); {
			err = a.client.PostAgentPorts(ctx, req)
			if err != nil {
				a.logger.Error(ctx, "failed to post agent ports", slog.Error(err))
				continue
			}
			break
		}
	}
}
