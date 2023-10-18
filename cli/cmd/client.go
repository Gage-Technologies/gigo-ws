package cmd

import (
	"context"
	"fmt"
	proto "github.com/gage-technologies/gigo-ws/protos/ws"
	"github.com/google/uuid"
	"net"
	"storj.io/drpc/drpcconn"
)

// TODO: add tests

type CreateWorkspaceOptions struct {
	WorkspaceID int64  `yaml:"workspace_id" json:"workspace_id"`
	OwnerID     int64  `yaml:"owner_id" json:"owner_id"`
	OwnerEmail  string `yaml:"owner_email" json:"owner_email"`
	OwnerName   string `yaml:"owner_name" json:"owner_name"`
	Disk        int    `yaml:"disk" json:"disk"`
	CPU         int    `yaml:"cpu" json:"cpu"`
	Memory      int    `yaml:"memory" json:"memory"`
	Container   string `yaml:"container" json:"container"`
	AccessUrl   string `yaml:"access_url" json:"access_url"`
}

type NewAgent struct {
	ID    int64
	Token uuid.UUID
}

type WorkspaceClientOptions struct {
	Host string
	Port int
}

type WorkspaceClient struct {
	conn   *drpcconn.Conn
	client proto.DRPCGigoWSClient
}

func NewWorkspaceClient(opts WorkspaceClientOptions) (*WorkspaceClient, error) {
	// dial server
	rawconn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", opts.Host, opts.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to dial remote server: %v", err)
	}

	// create a drpc connection
	conn := drpcconn.New(rawconn)

	// create new client
	client := proto.NewDRPCGigoWSClient(conn)

	return &WorkspaceClient{
		conn:   conn,
		client: client,
	}, nil
}

func (c *WorkspaceClient) Echo(ctx context.Context, echo string) (string, error) {
	// execute remote echo call
	res, err := c.client.Echo(ctx, &proto.EchoRequest{
		Echo: echo,
	})
	if err != nil {
		return "", fmt.Errorf("failed to perform echo: %v", err)
	}

	if res.GetStatus() != proto.ResponseCode_SUCCESS {
		return "", fmt.Errorf("failed to perform echo: %v", res.GetStatus().String())
	}

	return res.GetEcho(), nil
}

func (c *WorkspaceClient) CreateWorkspace(ctx context.Context, opts CreateWorkspaceOptions) (*NewAgent, error) {
	// create proto for request
	req := &proto.CreateWorkspaceRequest{
		WorkspaceId: opts.WorkspaceID,
		OwnerId:     opts.OwnerID,
		OwnerEmail:  opts.OwnerEmail,
		OwnerName:   opts.OwnerName,
		Disk:        int32(opts.Disk),
		Cpu:         int32(opts.CPU),
		Memory:      int32(opts.Memory),
		Container:   opts.Container,
		AccessUrl:   opts.AccessUrl,
	}

	// execute remote provision call
	res, err := c.client.CreateWorkspace(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %v", err)
	}

	// check status code
	if res.GetStatus() != proto.ResponseCode_SUCCESS {
		// handle go error
		if res.GetError() != nil && res.GetError().GetGoError() != "" {
			return nil, fmt.Errorf("remote server error creating workspace: %v", res.GetError().GetGoError())
		}

		// handle command error
		if res.GetError() != nil && res.GetError().GetCmdError() != nil {
			cmdErr := res.GetError().GetCmdError()
			return nil, fmt.Errorf(
				"remote command error creating workspace\n    status: %d\n    out: %s\n    err: %s",
				cmdErr.GetExitCode(), cmdErr.GetStdout(), cmdErr.GetStderr(),
			)
		}

		// handle unknown error
		return nil, fmt.Errorf("failed to create workspace: %v", res.GetStatus().String())
	}

	// ensure that agent id and token are present
	if res.GetAgentId() == 0 || res.GetAgentToken() == "" {
		return nil, fmt.Errorf("failed to create workspace: new agent data missing")
	}

	// format token to uuid
	tokenUuid, err := uuid.Parse(res.GetAgentToken())
	if err != nil {
		return nil, fmt.Errorf("failed to parse uuid: %v", err)
	}

	return &NewAgent{
		ID:    res.GetAgentId(),
		Token: tokenUuid,
	}, nil
}

func (c *WorkspaceClient) StartWorkspace(ctx context.Context, workspaceId int64) (*NewAgent, error) {
	// execute remote provision call
	res, err := c.client.StartWorkspace(ctx, &proto.StartWorkspaceRequest{
		WorkspaceId: workspaceId,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start workspace: %v", err)
	}

	// check status code
	if res.GetStatus() != proto.ResponseCode_SUCCESS {
		// handle go error
		if res.GetError() != nil && res.GetError().GetGoError() != "" {
			return nil, fmt.Errorf("remote server error start workspace: %v", res.GetError().GetGoError())
		}

		// handle command error
		if res.GetError() != nil && res.GetError().GetCmdError() != nil {
			cmdErr := res.GetError().GetCmdError()
			return nil, fmt.Errorf(
				"remote command error start workspace\n    status: %d\n    out: %s\n    err: %s",
				cmdErr.GetExitCode(), cmdErr.GetStdout(), cmdErr.GetStderr(),
			)
		}

		// handle unknown error
		return nil, fmt.Errorf("failed to start workspace: %v", res.GetStatus().String())
	}

	// ensure that agent id and token are present
	if res.GetAgentId() == 0 || res.GetAgentToken() == "" {
		return nil, fmt.Errorf("failed to start workspace: new agent data missing")
	}

	// format token to uuid
	tokenUuid, err := uuid.Parse(res.GetAgentToken())
	if err != nil {
		return nil, fmt.Errorf("failed to parse uuid: %v", err)
	}

	return &NewAgent{
		ID:    res.GetAgentId(),
		Token: tokenUuid,
	}, nil
}

func (c *WorkspaceClient) StopWorkspace(ctx context.Context, workspaceId int64) error {
	// execute remote provision call
	res, err := c.client.StopWorkspace(ctx, &proto.StopWorkspaceRequest{
		WorkspaceId: workspaceId,
	})
	if err != nil {
		return fmt.Errorf("failed to stop workspace: %v", err)
	}

	// check status code
	if res.GetStatus() != proto.ResponseCode_SUCCESS {
		// handle go error
		if res.GetError() != nil && res.GetError().GetGoError() != "" {
			return fmt.Errorf("remote server error stop workspace: %v", res.GetError().GetGoError())
		}

		// handle command error
		if res.GetError() != nil && res.GetError().GetCmdError() != nil {
			cmdErr := res.GetError().GetCmdError()
			return fmt.Errorf(
				"remote command error stop workspace\n    status: %d\n    out: %s\n    err: %s",
				cmdErr.GetExitCode(), cmdErr.GetStdout(), cmdErr.GetStderr(),
			)
		}

		// handle unknown error
		return fmt.Errorf("failed to start workspace: %v", res.GetStatus().String())
	}

	return nil
}

func (c *WorkspaceClient) DestroyWorkspace(ctx context.Context, workspaceId int64) error {
	// execute remote provision call
	res, err := c.client.DestroyWorkspace(ctx, &proto.DestroyWorkspaceRequest{
		WorkspaceId: workspaceId,
	})
	if err != nil {
		return fmt.Errorf("failed to destroy workspace: %v", err)
	}

	// check status code
	if res.GetStatus() != proto.ResponseCode_SUCCESS {
		// handle go error
		if res.GetError() != nil && res.GetError().GetGoError() != "" {
			return fmt.Errorf("remote server error destroy workspace: %v", res.GetError().GetGoError())
		}

		// handle command error
		if res.GetError() != nil && res.GetError().GetCmdError() != nil {
			cmdErr := res.GetError().GetCmdError()
			return fmt.Errorf(
				"remote command error destroy workspace\n    status: %d\n    out: %s\n    err: %s",
				cmdErr.GetExitCode(), cmdErr.GetStdout(), cmdErr.GetStderr(),
			)
		}

		// handle unknown error
		return fmt.Errorf("failed to destroy workspace: %v", res.GetStatus().String())
	}

	return nil
}
