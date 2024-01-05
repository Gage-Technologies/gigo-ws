package api

import (
	"context"
	"errors"
	"fmt"
	"gigo-ws/wspool"
	"net"
	"net/url"
	"sync"
	"time"

	"gigo-ws/config"
	"gigo-ws/volpool"

	"gigo-ws/protos/ws"
	"gigo-ws/provisioner"

	"github.com/bwmarrin/snowflake"
	"github.com/gage-technologies/drpc-lib/muxserver"
	"github.com/gage-technologies/gigo-lib/cluster"
	"github.com/gage-technologies/gigo-lib/logging"
	"github.com/gage-technologies/gigo-lib/storage"
	"k8s.io/client-go/kubernetes"
	"k8s.io/metrics/pkg/client/clientset/versioned"
	"storj.io/drpc/drpcmux"
)

const (
	ProvisionerJobPrefix = "provisioner/job/active"
)

type ProvisionerApiServerOptions struct {
	ID              int64
	ClusterNode     cluster.Node
	Provisioner     *provisioner.Provisioner
	Volpool         *volpool.VolumePool
	WsPool          *wspool.WorkspacePool
	StorageEngine   storage.Storage
	KubeClient      *kubernetes.Clientset
	MetricsClient   *versioned.Clientset
	SnowflakeNode   *snowflake.Node
	Host            string
	Port            int
	RegistryCaches  []config.RegistryCacheConfig
	WsHostOverrides map[string]string
	Logger          logging.Logger
}

// ProvisionerApiServer
//
//	DRPC api server to interact with provisioner remotely
type ProvisionerApiServer struct {
	ProvisionerApiServerOptions
	ctx      context.Context
	cancel   context.CancelFunc
	server   *muxserver.Server
	wg       *sync.WaitGroup
	Listener net.Listener
}

// NewProvisionerApiServer
//
//	Creates a new ProvisionerApiServer for serving workspace provisioning
//	to remote services
func NewProvisionerApiServer(options ProvisionerApiServerOptions) (*ProvisionerApiServer, error) {
	// create listener
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", options.Host, options.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to created listener: %v", err)
	}

	// create a new context for the provisioner server
	ctx, cancel := context.WithCancel(context.Background())
	s := &ProvisionerApiServer{
		ProvisionerApiServerOptions: options,
		ctx:                         ctx,
		cancel:                      cancel,
		wg:                          &sync.WaitGroup{},
		Listener:                    listener,
	}

	// we use DRPC form Storj because it is a simpler and faster
	// gRPC replacement

	// create a new muxer
	mux := drpcmux.New()

	// register the server implementation with DRPC
	err = ws.DRPCRegisterGigoWS(mux, s)
	if err != nil {
		return nil, fmt.Errorf("failed to register provisioner api server: %v", err)
	}

	// TODO: configure auth middleware
	handler := NewDrpcMiddleware(DrpcMiddlewareOptions{
		WaitGroup:     s.wg,
		Logger:        options.Logger,
		Handler:       mux,
		SnowflakeNode: options.SnowflakeNode,
	})

	// create a new server
	s.server = muxserver.New(handler)

	return s, nil
}

// Serve
//
//	Launches and serves the provisioner api
func (s *ProvisionerApiServer) Serve() error {
	// start the server
	return s.server.Serve(s.ctx, s.Listener)
}

// Close
//
//	Gracefully close the server by rejecting all new connections
//	and waiting for existing connections to finish.
func (s *ProvisionerApiServer) Close() error {
	// cancel the context to block any new connections from occurring
	s.cancel()
	// wait for active connections to finish
	s.wg.Wait()
	return nil
}

// Echo
//
//	Returns the echoed string
func (s *ProvisionerApiServer) Echo(ctx context.Context, req *ws.EchoRequest) (*ws.EchoResponse, error) {
	return &ws.EchoResponse{Echo: req.GetEcho()}, nil
}

// CreateWorkspace
//
//	Provisions a new workspace from scratch
func (s *ProvisionerApiServer) CreateWorkspace(ctx context.Context, request *ws.CreateWorkspaceRequest) (*ws.CreateWorkspaceResponse, error) {
	// perform validation on request
	err := validateCreateWorkspaceRequest(request)
	if err != nil {
		s.Logger.Warn(fmt.Errorf("CreateWorkspace (%d): failed to create workspace request: %v", ctx.Value("id"), err))
		return &ws.CreateWorkspaceResponse{
			Status: ws.ResponseCode_MALFORMED_REQUEST,
			Error: &ws.Error{
				GoError: err.Error(),
			},
		}, nil
	}

	s.Logger.Debug(fmt.Errorf("CreateWorkspace (%d): beginning workspace creation: %d", ctx.Value("id"), request.GetWorkspaceId()))

	// defer the removal of the provisioner job - if we fail or don't get the job
	// this will become a no-op
	defer func() {
		_ = removeProvisionerJob(s, request.GetWorkspaceId())
	}()

	// register provisioner job with the cluster
	ok, err := registerProvisionerJob(s, request.GetWorkspaceId())
	if err != nil {
		s.Logger.Warn(fmt.Errorf("CreateWorkspace (%d): failed to register provisioner job: %v", ctx.Value("id"), err))
		return &ws.CreateWorkspaceResponse{
			Status: ws.ResponseCode_SERVER_EXECUTION_ERROR,
			Error: &ws.Error{
				GoError: err.Error(),
			},
		}, nil
	}

	// handle the case that there is an active provisioner job
	if !ok {
		return &ws.CreateWorkspaceResponse{
			Status: ws.ResponseCode_ALTERNATIVE_REQUEST_ACTIVE,
		}, nil
	}

	// format request into createWorkspaceOptions
	opts := createWorkspaceOptions{
		Provisioner:   s.Provisioner,
		StorageEngine: s.StorageEngine,
		Logger:        s.Logger,
		TemplateOpts: templateOptions{
			WorkspaceID: request.GetWorkspaceId(),
			OwnerID:     request.GetOwnerId(),
			OwnerEmail:  request.GetOwnerEmail(),
			OwnerName:   request.GetOwnerName(),
			Disk:        int(request.GetDisk()),
			CPU:         int(request.GetCpu()),
			Memory:      int(request.GetMemory()),
			Container:   request.GetContainer(),
			AccessUrl:   request.GetAccessUrl(),
		},
		RegistryCaches:  s.RegistryCaches,
		WsHostOverrides: s.WsHostOverrides,
		Volpool:         s.Volpool,
		WsPool:          s.WsPool,
	}

	// perform workspace creation
	agent, _, err := createWorkspace(ctx, opts)
	if err != nil {
		s.Logger.Warn(fmt.Errorf("CreateWorkspace (%d): failed to create workspace: %v", ctx.Value("id"), err))
		return &ws.CreateWorkspaceResponse{
			Status: ws.ResponseCode_SERVER_EXECUTION_ERROR,
			Error: &ws.Error{
				GoError: err.Error(),
			},
		}, nil
	}

	// ensure agent is not nil
	if agent == nil {
		s.Logger.Warnf("CreateWorkspace (%d): agent was nil", ctx.Value("id"))
		return &ws.CreateWorkspaceResponse{
			Status: ws.ResponseCode_SERVER_EXECUTION_ERROR,
			Error: &ws.Error{
				GoError: "agent is nil",
			},
		}, nil
	}

	s.Logger.Debug(fmt.Errorf("CreateWorkspace (%d): completed workspace creation: %d", ctx.Value("id"), request.GetWorkspaceId()))

	// format agent and return
	return &ws.CreateWorkspaceResponse{
		Status:     ws.ResponseCode_SUCCESS,
		AgentId:    agent.ID,
		AgentToken: agent.Token,
	}, nil
}

// StartWorkspace
//
//	Starts an existing workspace that is currently stopped
func (s *ProvisionerApiServer) StartWorkspace(ctx context.Context, request *ws.StartWorkspaceRequest) (*ws.StartWorkspaceResponse, error) {
	// validate id
	if request.WorkspaceId < 1 {
		s.Logger.Warn(fmt.Errorf("StartWorkspace (%d): invalid workspace id: %d", ctx.Value("id"), request.GetWorkspaceId()))
		return &ws.StartWorkspaceResponse{
			Status: ws.ResponseCode_MALFORMED_REQUEST,
			Error: &ws.Error{
				GoError: "invalid workspace id",
			},
		}, nil
	}

	s.Logger.Debug(fmt.Errorf("StartWorkspace (%d): beginning workspace start: %d", ctx.Value("id"), request.GetWorkspaceId()))

	// defer the removal of the provisioner job - if we fail or don't get the job
	// this will become a no-op
	defer func() {
		_ = removeProvisionerJob(s, request.GetWorkspaceId())
	}()

	// register provisioner job with the cluster
	ok, err := registerProvisionerJob(s, request.GetWorkspaceId())
	if err != nil {
		s.Logger.Warn(fmt.Errorf("StartWorkspace (%d): failed to register provisioner job: %v", ctx.Value("id"), err))
		return &ws.StartWorkspaceResponse{
			Status: ws.ResponseCode_SERVER_EXECUTION_ERROR,
			Error: &ws.Error{
				GoError: err.Error(),
			},
		}, nil
	}

	// handle the case that there is an active provisioner job
	if !ok {
		return &ws.StartWorkspaceResponse{
			Status: ws.ResponseCode_ALTERNATIVE_REQUEST_ACTIVE,
		}, nil
	}

	// perform workspace stop
	agent, _, err := startWorkspace(ctx, startWorkspaceOptions{
		Provisioner:   s.Provisioner,
		StorageEngine: s.StorageEngine,
		Logger:        s.Logger,
		WorkspaceID:   request.GetWorkspaceId(),
	})
	if err != nil {
		s.Logger.Warn(fmt.Errorf("StartWorkspace (%d): failed to start workspace: %v", ctx.Value("id"), err))
		if errors.Is(err, ErrWorkspaceNotFound) {
			return &ws.StartWorkspaceResponse{
				Status: ws.ResponseCode_NOT_FOUND,
			}, nil
		}
		return &ws.StartWorkspaceResponse{
			Status: ws.ResponseCode_SERVER_EXECUTION_ERROR,
			Error: &ws.Error{
				GoError: err.Error(),
			},
		}, nil
	}

	s.Logger.Debug(fmt.Errorf("StartWorkspace (%d): completed workspace start: %d", ctx.Value("id"), request.GetWorkspaceId()))

	// format agent and return
	return &ws.StartWorkspaceResponse{
		Status:     ws.ResponseCode_SUCCESS,
		AgentId:    agent.ID,
		AgentToken: agent.Token,
	}, nil
}

// StopWorkspace
//
//	Stops a running workspace but preserves the terraform state
func (s *ProvisionerApiServer) StopWorkspace(ctx context.Context, request *ws.StopWorkspaceRequest) (*ws.StopWorkspaceResponse, error) {
	// validate id
	if request.WorkspaceId < 1 {
		s.Logger.Warn(fmt.Errorf("StopWorkspace (%d): invalid workspace id: %d", ctx.Value("id"), request.GetWorkspaceId()))
		return &ws.StopWorkspaceResponse{
			Status: ws.ResponseCode_MALFORMED_REQUEST,
			Error: &ws.Error{
				GoError: "invalid workspace id",
			},
		}, nil
	}

	s.Logger.Debug(fmt.Errorf("StopWorkspace (%d): beginning workspace stop: %d", ctx.Value("id"), request.GetWorkspaceId()))

	// defer the removal of the provisioner job - if we fail or don't get the job
	// this will become a no-op
	defer func() {
		_ = removeProvisionerJob(s, request.GetWorkspaceId())
	}()

	// register provisioner job with the cluster
	ok, err := registerProvisionerJob(s, request.GetWorkspaceId())
	if err != nil {
		s.Logger.Warn(fmt.Errorf("StopWorkspace (%d): failed to register provisioner job: %v", ctx.Value("id"), err))
		return &ws.StopWorkspaceResponse{
			Status: ws.ResponseCode_SERVER_EXECUTION_ERROR,
			Error: &ws.Error{
				GoError: err.Error(),
			},
		}, nil
	}

	// handle the case that there is an active provisioner job
	if !ok {
		return &ws.StopWorkspaceResponse{
			Status: ws.ResponseCode_ALTERNATIVE_REQUEST_ACTIVE,
		}, nil
	}

	// perform workspace stop
	_, _, err = stopWorkspace(ctx, stopWorkspaceOptions{
		Provisioner:   s.Provisioner,
		StorageEngine: s.StorageEngine,
		Logger:        s.Logger,
		WorkspaceID:   request.GetWorkspaceId(),
	})
	if err != nil {
		s.Logger.Warn(fmt.Errorf("StopWorkspace (%d): failed to stop workspace: %v", ctx.Value("id"), err))
		if errors.Is(err, ErrWorkspaceNotFound) {
			return &ws.StopWorkspaceResponse{
				Status: ws.ResponseCode_NOT_FOUND,
			}, nil
		}
		return &ws.StopWorkspaceResponse{
			Status: ws.ResponseCode_SERVER_EXECUTION_ERROR,
			Error: &ws.Error{
				GoError: err.Error(),
			},
		}, nil
	}

	s.Logger.Debug(fmt.Errorf("StopWorkspace (%d): completed workspace stop: %d", ctx.Value("id"), request.GetWorkspaceId()))

	return &ws.StopWorkspaceResponse{
		Status: ws.ResponseCode_SUCCESS,
	}, nil
}

// DestroyWorkspace
//
//	Destroys an existing workspace
func (s *ProvisionerApiServer) DestroyWorkspace(ctx context.Context, request *ws.DestroyWorkspaceRequest) (*ws.DestroyWorkspaceResponse, error) {
	// validate id
	if request.WorkspaceId < 1 {
		s.Logger.Warn(fmt.Errorf("DestroyWorkspace (%d): invalid workspace id: %d", ctx.Value("id"), request.GetWorkspaceId()))
		return &ws.DestroyWorkspaceResponse{
			Status: ws.ResponseCode_MALFORMED_REQUEST,
			Error: &ws.Error{
				GoError: "invalid workspace id",
			},
		}, nil
	}

	s.Logger.Debug(fmt.Errorf("DestroyWorkspace (%d): beginning workspace destroy: %d", ctx.Value("id"), request.GetWorkspaceId()))

	// defer the removal of the provisioner job - if we fail or don't get the job
	// this will become a no-op
	defer func() {
		_ = removeProvisionerJob(s, request.GetWorkspaceId())
	}()

	// register provisioner job with the cluster
	ok, err := registerProvisionerJob(s, request.GetWorkspaceId())
	if err != nil {
		s.Logger.Warn(fmt.Errorf("DestroyWorkspace (%d): failed to register provisioner job: %v", ctx.Value("id"), err))
		return &ws.DestroyWorkspaceResponse{
			Status: ws.ResponseCode_SERVER_EXECUTION_ERROR,
			Error: &ws.Error{
				GoError: err.Error(),
			},
		}, nil
	}

	// handle the case that there is an active provisioner job
	if !ok {
		return &ws.DestroyWorkspaceResponse{
			Status: ws.ResponseCode_ALTERNATIVE_REQUEST_ACTIVE,
		}, nil
	}

	// perform workspace stop
	_, err = destroyWorkspace(ctx, destroyWorkspaceOptions{
		Provisioner:   s.Provisioner,
		StorageEngine: s.StorageEngine,
		Logger:        s.Logger,
		WorkspaceID:   request.GetWorkspaceId(),
		Volpool:       s.Volpool,
	})
	if err != nil {
		// we treat a not-found as an idempotent destroy since we want it gone anyway
		if errors.Is(err, ErrWorkspaceNotFound) {
			return &ws.DestroyWorkspaceResponse{
				Status: ws.ResponseCode_SUCCESS,
			}, nil
		}
		s.Logger.Warn(fmt.Errorf("DestroyWorkspace (%d): failed to destroy workspace: %v", ctx.Value("id"), err))
		return &ws.DestroyWorkspaceResponse{
			Status: ws.ResponseCode_SERVER_EXECUTION_ERROR,
			Error: &ws.Error{
				GoError: err.Error(),
			},
		}, nil
	}

	s.Logger.Debug(fmt.Errorf("DestroyWorkspace (%d): completed workspace destroy: %d", ctx.Value("id"), request.GetWorkspaceId()))

	return &ws.DestroyWorkspaceResponse{
		Status: ws.ResponseCode_SUCCESS,
	}, nil
}

// GetResourceUtil
//
//	Retrieves the resource utilization of a workspace
func (s *ProvisionerApiServer) GetResourceUtil(ctx context.Context, request *ws.GetResourceUtilRequest) (*ws.GetResourceUtilResponse, error) {
	// validate id
	if request.WorkspaceId < 1 {
		s.Logger.Warn(fmt.Errorf("GetResourceUtilResponse (%d): invalid workspace id: %d", ctx.Value("id"), request.GetWorkspaceId()))
		return &ws.GetResourceUtilResponse{
			Status: ws.ResponseCode_MALFORMED_REQUEST,
			Error: &ws.Error{
				GoError: "invalid workspace id",
			},
		}, nil
	}

	s.Logger.Debug(fmt.Errorf("GetResourceUtilResponse (%d): beginning retrieve resource util: %d", ctx.Value("id"), request.GetWorkspaceId()))

	// perform workspace stop
	util, err := getResourceUtil(ctx, getResourceUtilOptions{
		KubeClient:    s.KubeClient,
		MetricsClient: s.MetricsClient,
		WorkspaceID:   request.GetWorkspaceId(),
		OwnerID:       request.GetOwnerId(),
	})
	if err != nil {
		s.Logger.Warn(fmt.Errorf("GetResourceUtilResponse (%d): failed to retrieve resource util: %v", ctx.Value("id"), err))
		return &ws.GetResourceUtilResponse{
			Status: ws.ResponseCode_SERVER_EXECUTION_ERROR,
			Error: &ws.Error{
				GoError: err.Error(),
			},
		}, nil
	}

	s.Logger.Debug(fmt.Errorf("GetResourceUtilResponse (%d): completed retrieve resource util: %d", ctx.Value("id"), request.GetWorkspaceId()))

	return &ws.GetResourceUtilResponse{
		Status:      ws.ResponseCode_SUCCESS,
		Cpu:         util.CPU,
		Memory:      util.Memory,
		CpuLimit:    util.CPULimit,
		MemoryLimit: util.MemoryLimit,
		CpuUsage:    util.CPUUsage,
		MemoryUsage: util.MemoryUsage,
	}, nil
}

// validateCreateWorkspaceRequest
//
//	Helper function to validate ws.CreateWorkspaceRequest
func validateCreateWorkspaceRequest(request *ws.CreateWorkspaceRequest) error {
	if request.GetWorkspaceId() < 1 {
		return fmt.Errorf("invalid workspace id")
	}

	if request.GetOwnerId() < 1 {
		return fmt.Errorf("invalid owner id")
	}

	if request.GetOwnerEmail() == "" {
		return fmt.Errorf("invalid owner email")
	}

	if request.GetOwnerName() == "" {
		return fmt.Errorf("invalid owner name")
	}

	if request.GetDisk() < 5 || request.GetDisk() > 250 {
		return fmt.Errorf("invalid disk - must be 5 <= x <= 250")
	}

	if request.GetCpu() < 2 || request.GetCpu() > 32 {
		return fmt.Errorf("invalid cpu - must be 2 <= x <= 32")
	}

	if request.GetMemory() < 2 || request.GetMemory() > 32 {
		return fmt.Errorf("invalid memory - must be 2 <= x <= 32")
	}

	if request.GetContainer() == "" {
		return fmt.Errorf("invalid container")
	}

	if _, err := url.Parse(request.GetAccessUrl()); err != nil {
		return fmt.Errorf("invalid access url: %v", err)
	}

	return nil
}

// registerProvisionerJob
//
//		Registers an active provisioner job with the cluster bound to this node.
//	 This function will return true if the job was accepted and registered and false
//	 if there is currently an active job for the same workspace.
func registerProvisionerJob(s *ProvisionerApiServer, workspaceId int64) (bool, error) {
	// TODO: handle the race condition of another node/caller acquiring the job between the
	// job check and the registration.

	// check for an active provisioner job in the cluster
	activeJobs, err := s.ClusterNode.GetCluster(fmt.Sprintf("%s/%d", ProvisionerJobPrefix, workspaceId))
	if err != nil {
		return false, fmt.Errorf("failed to get active provisioner job: %v", err)
	}
	active := false
	for _, kvs := range activeJobs {
		if len(kvs) > 0 {
			active = true
			break
		}
	}

	// return false to indicate that there is currently an active provisioner job
	if active {
		return false, nil
	}

	// insert a new provisioner job under this node
	// we set the key's value to the time we started as a debug
	// measure incase we ever need to know how long the job took
	err = s.ClusterNode.Put(
		fmt.Sprintf("%s/%d", ProvisionerJobPrefix, workspaceId),
		fmt.Sprintf("%d", time.Now().Unix()),
	)
	if err != nil {
		return false, fmt.Errorf("failed to register workspace task: %v", err)
	}

	return true, nil
}

// removeProvisionerJob
//
//	Removes an active provisioner job with the cluster bound to this node.
func removeProvisionerJob(s *ProvisionerApiServer, workspaceId int64) error {
	// remove the provisioner job from this node in the cluster
	err := s.ClusterNode.Delete(fmt.Sprintf("%s/%d", ProvisionerJobPrefix, workspaceId))
	if err != nil {
		return fmt.Errorf("failed to remove workspace task from cluster node: %v", err)
	}
	return nil
}
