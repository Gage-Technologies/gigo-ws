package provisioner

import (
	"fmt"
	"io"
	"strconv"

	"github.com/buger/jsonparser"
	"github.com/gage-technologies/gigo-ws/models"
	"github.com/gage-technologies/gigo-ws/provisioner/backend"
)

// ParseStatefileForAgent
//
//	 Parses a terraform state file and returns the gigo_agent's
//		id and token
func ParseStatefileForAgent(provisionerBackend backend.ProvisionerBackend, workspaceId int64) (*models.Agent, error) {
	// retrieve state file from storage engine
	buf, err := provisionerBackend.GetStatefile(fmt.Sprintf("states/%d", workspaceId))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve statefile: %v", err)
	}
	defer buf.Close()

	// read state file
	stateBuf, err := io.ReadAll(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read statefile: %v", err)
	}

	// close buffer
	_ = buf.Close()

	// create agent variable to hold parsed agent data
	var agent *models.Agent

	// retrieve resources
	resourcesBuf, resourcesType, _, err := jsonparser.Get(stateBuf, "resources")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve resources: %v", err)
	}

	// ensure that resources is an array
	if resourcesType != jsonparser.Array {
		return nil, fmt.Errorf("resources is not an array")
	}

	// parse the state file for the agent
	jsonparser.ArrayEach(resourcesBuf, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		// fmt.Println(string(value))

		// skip non object - this should never occur
		if dataType != jsonparser.Object {
			return
		}

		// parse type from resource
		resourceType, err := jsonparser.GetString(value, "type")
		if err != nil {
			return
		}

		// ensure resource type is "agent"
		if string(resourceType) != "gigo_agent" {
			return
		}

		// attempt to parse id from resource
		idString, err := jsonparser.GetString(value, "instances", "[0]", "attributes", "id")
		if err != nil {
			return
		}

		// attempt to parse id to integer
		id, err := strconv.ParseInt(idString, 10, 64)
		if err != nil {
			return
		}

		// attempt to parse token from resource
		tokenString, err := jsonparser.GetString(value, "instances", "[0]", "attributes", "token")
		if err != nil {
			return
		}

		// set agent
		agent = &models.Agent{
			ID:    id,
			Token: tokenString,
		}
	})

	// return error if agent wasn't found
	if agent == nil {
		return nil, fmt.Errorf("agent not found")
	}

	return agent, nil
}

// ParseStatefileForWorkspaceState
//
//	Parses a terraform state file and returns the workspace state
func ParseStatefileForWorkspaceState(provisionerBackend backend.ProvisionerBackend, workspaceId int64) (models.WorkspaceState, error) {
	// retrieve state file from storage engine
	buf, err := provisionerBackend.GetStatefile(fmt.Sprintf("states/%d", workspaceId))
	if err != nil {
		return -1, fmt.Errorf("failed to retrieve statefile: %v", err)
	}

	// return destroyed if statefile is not found
	if buf == nil {
		return models.WorkspaceStateDestroyed, nil
	}

	defer buf.Close()

	// read state file
	stateBuf, err := io.ReadAll(buf)
	if err != nil {
		return -1, fmt.Errorf("failed to read statefile: %v", err)
	}

	// close buffer
	_ = buf.Close()

	// create state variable to hold parsed state
	state := models.WorkspaceState(-1)

	// retrieve resources
	resourcesBuf, resourcesType, _, err := jsonparser.Get(stateBuf, "resources")
	if err != nil {
		return -1, fmt.Errorf("failed to retrieve resources: %v", err)
	}

	// ensure that resources is an array
	if resourcesType != jsonparser.Array {
		return -1, fmt.Errorf("resources is not an array")
	}

	// parse the state file for the agent
	jsonparser.ArrayEach(resourcesBuf, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		// fmt.Println(string(value))

		// skip non object - this should never occur
		if dataType != jsonparser.Object {
			return
		}

		// parse type from resource
		resourceType, err := jsonparser.GetString(value, "type")
		if err != nil {
			return
		}

		// ensure resource type is "agent"
		if string(resourceType) != "gigo_workspace" {
			return
		}

		// attempt to parse start count from resource
		startCount, err := jsonparser.GetInt(value, "instances", "[0]", "attributes", "start_count")
		if err != nil {
			return
		}

		// set workspace state
		if startCount > 0 {
			state = models.WorkspaceStateActive
		} else {
			state = models.WorkspaceStateStopped
		}
	})

	// return destroyed if state wasn't found
	if state == -1 {
		return models.WorkspaceStateDestroyed, nil
	}

	return state, nil
}
