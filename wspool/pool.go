package wspool

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"gigo-ws/utils"
	"os"
	"runtime/debug"
	"strings"

	"gigo-ws/config"
	models2 "gigo-ws/models"
	"gigo-ws/provisioner"

	"github.com/bwmarrin/snowflake"
	ti "github.com/gage-technologies/gigo-lib/db"
	"github.com/gage-technologies/gigo-lib/db/models"
	"github.com/gage-technologies/gigo-lib/logging"
	"github.com/gage-technologies/gigo-lib/storage"
	"github.com/sourcegraph/conc/pool"
	"golang.org/x/sync/singleflight"
)

//go:embed resources
var embedFS embed.FS

const hostAliasesTemplate = `
    host_aliases {
      ip = "%s"
      hostnames = ["%s"]
    }
`

type WorkspacePoolParams struct {
	// DB Database used to store the Workspace pool state
	DB *ti.Database

	// Provisioner TF Provisioner used to provision the Workspaces
	Provisioner *provisioner.Provisioner

	// StorageEngine Storage engine used to store the terraform modules
	StorageEngine storage.Storage

	// SfNode Snowflake node to generate ids
	SfNode *snowflake.Node

	// Logger Logger used to log messages
	Logger logging.Logger

	// Config WorkspacePool configuration
	Config config.WsPoolConfig

	WsHostOverrides map[string]string

	RegistryCaches []config.RegistryCacheConfig
}

type WorkspacePool struct {
	WorkspacePoolParams
	sflight singleflight.Group
}

func NewWorkspacePool(params WorkspacePoolParams) *WorkspacePool {
	pool := &WorkspacePool{
		WorkspacePoolParams: params,
	}

	return pool
}

// GetWorkspace
//
//		Attempts to retrieve a Workspace from the pool. If a Workspace is not available,
//		nil is returned.
//	 The Workspace is immediately claimed for the workspace id provided. If the caller
//	 aborts the workspace creation, the Workspace should be released back to the pool.
func (p *WorkspacePool) GetWorkspace(container string, memory int64, cpu int64, volumeSize int, workspaceId int64) (*models.WorkspacePool, error) {
	// start tx
	tx, err := p.DB.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("error beginning transaction: %v", err)
	}
	defer tx.Rollback()

	// query for the first available Workspace of the requested size
	res, err := tx.Query(
		"select _id, container, state, memory, cpu, volume_size, bin_to_uuid(secret) as secret, agent_id, workspace_table_id from workspace_pool where container = ? and state = ? and memory = ? and cpu = ? and volume_size = ? limit 1 for update",
		container, models.WorkspacePoolStateAvailable, memory, cpu, volumeSize,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying for available Workspaces: %v", err)
	}
	defer res.Close()

	// if no Workspaces are available, return nil
	if !res.Next() {
		return nil, nil
	}

	// load the Workspace
	ws, err := models.WorkspacePoolFromSqlNative(res)
	if err != nil {
		return nil, fmt.Errorf("error loading Workspace: %v", err)
	}
	_ = res.Close()

	// update the Workspace state
	ws.State = models.WorkspacePoolStateInUse

	// update the Workspace workspace id
	ws.WorkspaceTableID = &workspaceId

	// update the Workspace in the database
	_, err = tx.Exec("update workspace_pool set state = ?, workspace_table_id = ? where _id = ?", ws.State, ws.WorkspaceTableID, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("error updating Workspace: %v", err)
	}

	// commit the transaction
	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("error committing transaction: %v", err)
	}

	return ws, nil
}

// ReleaseWorkspace
//
//	Releases a Workspace back to the pool.
//	This should be called when a workspace creation is aborted.
//	NEVER RELEASE A Workspace WHICH HAS BEEN ATTACHED TO A WORKSPACE - WorkspaceS CONTAIN USER DATA
func (p *WorkspacePool) ReleaseWorkspace(workspacePoolId int64) error {
	// update the Workspace state in the database
	_, err := p.DB.DB.Exec("update workspace_pool set state = ?, workspace_table_id = null where _id = ?", models.WorkspacePoolStateAvailable, workspacePoolId)
	if err != nil {
		return fmt.Errorf("error updating Workspace: %v", err)
	}
	return nil
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////// ASK SAM
// DestroyWorkspaceWorkspaces
//
//	Destroys all Workspaces associated with the passed workspace id.
//	 This should be called when a workspace is destroyed.
func (p *WorkspacePool) DestroyWorkspacePoolByTableID(workspaceId int64) (bool, error) {
	// query for the Workspaces associated with the workspace
	res, err := p.DB.DB.Query("select * from workspace_pool where workspace_table_id = ?", workspaceId)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return true, fmt.Errorf("error querying for workspace Workspaces: %v", err)
	}
	defer res.Close()

	// create slice to hold the Workspaces
	vols := make([]*models.WorkspacePool, 0)

	// iterate over the results
	for res.Next() {
		// load the Workspace
		vol, err := models.WorkspacePoolFromSqlNative(res)
		if err != nil {
			return true, fmt.Errorf("error loading Workspace: %v", err)
		}

		// append the Workspace to the slice
		vols = append(vols, vol)
	}

	_ = res.Close()

	// iterate over the Workspaces and destroy them
	for _, vol := range vols {
		err := p.destroyWorkspace(vol.ID)
		if err != nil {
			return true, fmt.Errorf("error destroying Workspace: %v", err)
		}
	}

	return true, nil
}

func (p *WorkspacePool) GetPoolAliasID(workspaceTableId int64) (int64, error) {
	// check if the workspace table id exists in the database
	var id int64
	err := p.DB.DB.QueryRow("select _id from workspace_pool where workspace_table_id = ?", workspaceTableId).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("error querying for workspace Workspaces: %v", err)
	}
	return id, nil
}

// ResolveStateDeltas
//
//	Public method to resolve state deltas which can be called from outside the pool.
//	Wraps the call in a singleflight group to prevent multiple calls from being made
//	simultaneously.
func (p *WorkspacePool) ResolveStateDeltas() {
	p.sflight.Do("resolveStateDeltas", func() (interface{}, error) {
		defer func() {
			if err := recover(); err != nil {
				p.Logger.Errorf("panic resolving state deltas: %v\nstack:\n%s", err, string(debug.Stack()))
			}
		}()

		p.resolveStateDeltas()
		return nil, nil
	})
}

// resolveStateDeltas
//
//	Compares the configuration against the current state of the Workspace pool
//	across the cluster and determines what actions need to be taken to
//	reconcile the two.
func (p *WorkspacePool) resolveStateDeltas() {
	p.Logger.Debug("resolving state deltas")

	// create slices of Workspaces that should be provisioned and destroyed
	//provisionSet := make([]*models.WorkspacePool, 0)
	destroySet := make([]int64, 0)

	// iterate over the subpools
	for _, subpool := range p.Config.SubPools {
		p.Logger.Debugf("resolving state deltas for subpool: %s", subpool.Container)

		// get the count of available Workspaces in the subpool
		var availableCount int
		err := p.DB.DB.QueryRow(
			"select count(*) from workspace_pool where container = ? and state = ? and memory = ? and cpu = ? and volume_size =?",
			subpool.Container, models.WorkspacePoolStateAvailable, subpool.Memory, subpool.CPU, subpool.VolumeSize,
		).Scan(&availableCount)
		if err != nil {
			p.Logger.Errorf("error querying for available Workspaces for subpool %d: %v", subpool.Container, err)
			continue
		}

		// if the count of available Workspaces is less than the pool size, we need to provision more
		if availableCount < subpool.PoolSize {
			// calculate the number of Workspaces we need to provision
			neededCount := subpool.PoolSize - availableCount

			p.Logger.Debugf("provisioning %d Workspaces for subpool %d", neededCount, subpool.Container)

			// create worker pool to launch workspace creations concurrently
			wg := pool.New().WithMaxGoroutines(10)

			// create the Workspaces
			for i := 0; i < neededCount; i++ {
				wg.Go(func() {
					id := p.SfNode.Generate().Int64()
					pooledWs := models.CreateWorkspacePool(
						id,
						subpool.Container,
						models.WorkspacePoolStateAvailable,
						subpool.Memory,
						subpool.CPU,
						subpool.VolumeSize,
						"",
						subpool.StorageClass,
						nil,
					)

					p.Logger.Debugf("provisioning wspool Workspace: %d", pooledWs.ID)
					agent, _, err := p.provisionWorkspace(context.TODO(), pooledWs)
					if err != nil {
						p.Logger.Errorf("error provisioning Workspace pool %d: %v", pooledWs.ID, err)
						return
					}

					pooledWs.Secret = agent.Token
					pooledWs.AgentID = agent.ID
					statements, err := pooledWs.ToSqlNative()
					if err != nil {
						p.Logger.Errorf("error converting Workspace pool to SQL: %v", err)
						return
					}

					tx, err := p.DB.DB.BeginTx(context.Background(), nil)
					if err != nil {
						p.Logger.Error("error beginning transaction: %v", err)
						return
					}
					defer tx.Rollback()

					for _, statement := range statements {
						_, err = tx.Exec(statement.Statement, statement.Values...)
						if err != nil {
							p.Logger.Errorf("error inserting Workspace pool: %v", err)
							return
						}
					}

					err = tx.Commit()
					if err != nil {
						p.Logger.Error("error committing transaction: %v", err)
						return
					}
				})
			}

			// wait for all of the creations to complete
			wg.Wait()
		}

		// if the count of available Workspaces is greater than the pool size, we need to destroy some
		if availableCount > subpool.PoolSize {
			// query for the count of Workspaces that are available and not in use
			res, err := p.DB.DB.Query(
				"select _id from where container = ? and state = ? and memory = ? and cpu = ? and volume_size = ? limit ?",
				subpool.Container, models.WorkspacePoolStateAvailable, subpool.Memory, subpool.CPU, subpool.VolumeSize, availableCount-subpool.PoolSize,
			)
			if err != nil {
				p.Logger.Errorf("error querying for available Workspaces for subpool %d: %v", subpool.Container, err)
				continue
			}

			// iterate over the results and add the ids to the destroy set
			for res.Next() {
				var id int64
				err := res.Scan(&id)
				if err != nil {
					p.Logger.Errorf("error scanning row for available Workspaces for subpool %d: %v", subpool.Container, err)
					continue
				}

				destroySet = append(destroySet, id)
			}

			_ = res.Close()

			p.Logger.Debugf("destroying %d Workspaces for subpool %d", len(destroySet), subpool.Container)
		}
	}

	// destroy the Workspaces that need to be destroyed
	for _, id := range destroySet {
		p.Logger.Debugf("destroying pool Workspace: %d", id)
		err := p.destroyWorkspace(id)
		if err != nil {
			p.Logger.Errorf("error destroying Workspace %d: %v", id, err)
		}
	}
}

func (p *WorkspacePool) provisionWorkspace(ctx context.Context, pool *models.WorkspacePool) (*models2.Agent, *provisioner.ApplyLogs, error) {
	// retrieve the current state from statefile
	state, err := provisioner.ParseStatefileForWorkspaceState(p.Provisioner.Backend, pool.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse workspace state from statefile: %v", err)
	}

	// return error if the current workspace state is anything other than destroyed
	// since that would indicate that the workspace is already created
	if state != models2.WorkspaceStateDestroyed {
		return nil, nil, fmt.Errorf("workspace has already been created")
	}

	// ensure that the state file is removed incase it exists
	_ = p.Provisioner.Backend.RemoveStatefile(fmt.Sprintf("states/%d", pool.ID))

	// read template from storage
	templateBuf, err := embedFS.ReadFile("resources/template_vol.tf")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read tf template: %v", err)
	}

	// conditionally inject host aliases into template
	hostAliases := ""
	if len(p.WsHostOverrides) > 0 {
		for host, ip := range p.WsHostOverrides {
			hostAliases += fmt.Sprintf(hostAliasesTemplate, ip, host)
		}
		templateBuf = []byte(strings.ReplaceAll(string(templateBuf), "<HOST_ALIASES>", hostAliases))
	}
	templateBuf = []byte(strings.ReplaceAll(string(templateBuf), "<HOST_ALIASES>", hostAliases))

	// conditionally set the storage class
	sclass := ""
	if pool.StorageClass != "" {
		sclass = fmt.Sprintf("storage_class_name = \"%s\"", pool.StorageClass)
	}
	templateBuf = []byte(strings.ReplaceAll(string(templateBuf), "<STORAGE_CLASS>", sclass))

	// update the container with registry caching if it is configured
	container := utils.HandleRegistryCaches(pool.Container, p.RegistryCaches)

	// format module with terraform template
	module := &models2.TerraformModule{
		MainTF: templateBuf,
		////////////////////////////////////// TODO ASK Sam
		ModuleID:    pool.ID,
		Environment: p.prepEnvironmentForCreation(pool, container),
	}

	// create boolean to track failure
	failed := true

	// TODO: this seems a bit optimistic for cleanup - evaluate if this orphans resources
	// defer cleanup function to destroy resource on failure
	defer func() {
		if failed {
			// use a new context here since we don't want this interrupted by
			// drpc api call context - the api context could be cancelled
			// mid-operation but we want this to complete async
			_, err := p.Provisioner.Destroy(context.TODO(), module)
			if err != nil {
				p.Logger.Error(fmt.Errorf("failed to destroy workspace on create cleanup: %v", err))
			}
		}

		// // clean up the temporary module on fs
		// err := os.RemoveAll(module.LocalPath)
		// if err != nil {
		// 	opts.Logger.Error(fmt.Errorf("failed to clean up temporary module on create cleanup: %v", err))
		// }
		return
	}()

	// perform apply operation
	logs, err := p.Provisioner.Apply(ctx, module)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply configuration: %v", err)
	}

	// retrieve agent from statefile
	agent, err := provisioner.ParseStatefileForAgent(p.Provisioner.Backend, pool.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse agent from statefile: %v", err)
	}

	// preserve module for later operations
	err = module.StoreModule(p.StorageEngine)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to store module: %v", err)
	}

	// mark operation as a success to prevent cleanup
	failed = false

	// return apply logs
	return agent, logs, nil
}

func (p *WorkspacePool) prepEnvironmentForCreation(pool *models.WorkspacePool, formattedContainer string) []string {
	// initialize environment with our current environment
	// this is really important for k8s deployment because
	// the k8s api server config is in the container's environment
	// and we won't be able to provision unless we have access to
	// that configuration within the provisioner commands
	env := os.Environ()

	// format parameters into environment variables so that
	// terraform can compute the provisioning state
	env = append(env,
		fmt.Sprintf("GIGO_WORKSPACE_OWNER=%s", p.Config.OwnerName),
		fmt.Sprintf("GIGO_WORKSPACE_OWNER_EMAIL=%s", p.Config.OwnerEmail),
		fmt.Sprintf("GIGO_WORKSPACE_OWNER_ID=%d", p.Config.OwnerID),
		fmt.Sprintf("GIGO_WORKSPACE_DISK=%dGi", pool.VolumeSize),
		fmt.Sprintf("GIGO_WORKSPACE_CPU=%d", pool.CPU),
		fmt.Sprintf("GIGO_WORKSPACE_MEM=%dG", pool.Memory),
		fmt.Sprintf("GIGO_WORKSPACE_CONTAINER=%s", formattedContainer),
		fmt.Sprintf("GIGO_AGENT_URL=%s", p.Config.AccessUrl),
		///////////////////////////////////////////////////////////////////////////// TODO ASK Sam
		fmt.Sprintf("GIGO_WORKSPACE_ID=%d", pool.ID),
		"GIGO_WORKSPACE_TRANSITION=start",
	)

	// add agent scripts to the environments
	env = append(env, utils.AgentScriptEnv()...)

	return env
}

func (p *WorkspacePool) destroyWorkspace(poolId int64) error {
	// load module using the workspace id
	module, err := models2.LoadModule(p.StorageEngine, poolId)
	if err != nil {
		return fmt.Errorf("failed to load module: %v", err)
	}
	if module == nil {
		// if it's not found then we don't need to do anything
		return nil
	}

	// perform destroy operation
	_, err = p.Provisioner.Destroy(context.Background(), module)
	if err != nil {
		return fmt.Errorf("failed to destroy configuration: %v", err)
	}

	// delete the module from storage
	err = models2.DeleteModule(p.StorageEngine, poolId)
	if err != nil {
		return fmt.Errorf("failed to delete module from storage: %v", err)
	}

	// delete statefiles
	err = p.Provisioner.Backend.RemoveStatefile(fmt.Sprintf("states/%d", poolId))
	if err != nil {
		return fmt.Errorf("failed to remove terraform statefiles: %v", err)
	}

	// delete the Workspace from the database
	_, err = p.DB.DB.Exec("delete from workspace_pool where _id = ?", poolId)
	if err != nil {
		return fmt.Errorf("failed to delete Workspace from database: %v", err)
	}

	return nil
}
