package provisioner

import (
	config2 "github.com/gage-technologies/gigo-lib/config"
	"github.com/gage-technologies/gigo-ws/models"
	"github.com/gage-technologies/gigo-ws/provisioner/backend"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseStatefileForAgent(t *testing.T) {
	_, b, _, _ := runtime.Caller(0)
	basepath := strings.Replace(filepath.Dir(b), "/provisioner", "", -1)
	pb, err := backend.NewProvisionerBackendFS(config2.StorageFSConfig{
		Root: basepath + "/test_data/statefiles",
	})
	if err != nil {
		t.Fatal(err)
	}

	agent, err := ParseStatefileForAgent(pb, 420)
	if err != nil {
		t.Fatal(err)
	}

	if agent == nil {
		t.Fatal("agent is nil")
	}

	if agent.ID != 1620436895550922752 {
		t.Fatal("agent id is not 1620436895550922752")
	}

	if agent.Token != "8d6cadd6-3469-4ff8-9543-a92a6b160c64" {
		t.Fatal("agent token is not 8d6cadd6-3469-4ff8-9543-a92a6b160c64")
	}
}

func TestParseStatefileForWorkspaceState(t *testing.T) {
	_, b, _, _ := runtime.Caller(0)
	basepath := strings.Replace(filepath.Dir(b), "/provisioner", "", -1)
	pb, err := backend.NewProvisionerBackendFS(config2.StorageFSConfig{
		Root: basepath + "/test_data/statefiles",
	})
	if err != nil {
		t.Fatal(err)
	}

	state, err := ParseStatefileForWorkspaceState(pb, 420)
	if err != nil {
		t.Fatal(err)
	}

	if state != models.WorkspaceStateActive {
		t.Fatal("state is invalid: ", state)
	}

	state, err = ParseStatefileForWorkspaceState(pb, 421)
	if err != nil {
		t.Fatal(err)
	}

	if state != models.WorkspaceStateStopped {
		t.Fatal("state is invalid: ", state)
	}

	state, err = ParseStatefileForWorkspaceState(pb, 422)
	if err != nil {
		t.Fatal(err)
	}

	if state != models.WorkspaceStateDestroyed {
		t.Fatal("state is invalid: ", state)
	}
}
