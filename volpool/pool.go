package volpool

import (
	"context"
	"embed"
	"fmt"
	"os"
	"strings"

	"gigo-ws/config"
	models2 "gigo-ws/models"
	"gigo-ws/provisioner"

	"github.com/bwmarrin/snowflake"
	ti "github.com/gage-technologies/gigo-lib/db"
	"github.com/gage-technologies/gigo-lib/db/models"
	"github.com/gage-technologies/gigo-lib/logging"
	"github.com/gage-technologies/gigo-lib/storage"
	"golang.org/x/sync/singleflight"
)

//go:embed resources
var embedFS embed.FS

type VolumePoolParams struct {
	// DB Database used to store the volume pool state
	DB *ti.Database

	// Provisioner TF Provisioner used to provision the volumes
	Provisioner *provisioner.Provisioner

	// StorageEngine Storage engine used to store the terraform modules
	StorageEngine storage.Storage

	// SfNode Snowflake node to generate ids
	SfNode *snowflake.Node

	// Logger Logger used to log messages
	Logger logging.Logger

	// Config VolumePool configuration
	Config config.VolumePoolConfig
}

type VolumePool struct {
	VolumePoolParams
	sflight singleflight.Group
}

func NewVolumePool(params VolumePoolParams) *VolumePool {
	pool := &VolumePool{
		VolumePoolParams: params,
	}

	return pool
}

// GetVolume
//
//		Attempts to retrieve a volume from the pool. If a volume is not available,
//		nil is returned.
//	 The volume is immediately claimed for the workspace id provided. If the caller
//	 aborts the workspace creation, the volume should be released back to the pool.
func (p *VolumePool) GetVolume(size int64, workspaceId int64) (*models.VolpoolVolume, error) {
	// start tx
	tx, err := p.DB.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("error beginning transaction: %v", err)
	}
	defer tx.Rollback()

	// query for the first available volume of the requested size
	res, err := tx.Query(
		"select * from volpool_volume where size = ? and state = ? limit 1 for update",
		size, models.VolumeStateAvailable,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying for available volumes: %v", err)
	}
	defer res.Close()

	// if no volumes are available, return nil
	if !res.Next() {
		return nil, nil
	}

	// load the volume
	vol, err := models.VolpoolVolumeFromSqlNative(res)
	if err != nil {
		return nil, fmt.Errorf("error loading volume: %v", err)
	}
	_ = res.Close()

	// update the volume state
	vol.State = models.VolumeStateInUse

	// update the volume workspace id
	vol.WorkspaceID = &workspaceId

	// update the volume in the database
	_, err = tx.Exec("update volpool_volume set state = ?, workspace_id = ? where _id = ?", vol.State, vol.WorkspaceID, vol.ID)
	if err != nil {
		return nil, fmt.Errorf("error updating volume: %v", err)
	}

	// commit the transaction
	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("error committing transaction: %v", err)
	}

	return vol, nil
}

// ReleaseVolume
//
//	Releases a volume back to the pool.
//	This should be called when a workspace creation is aborted.
//	NEVER RELEASE A VOLUME WHICH HAS BEEN ATTACHED TO A WORKSPACE - VOLUMES CONTAIN USER DATA
func (p *VolumePool) ReleaseVolume(voldId int64) error {
	// update the volume state in the database
	_, err := p.DB.DB.Exec("update volpool_volume set state = ?, workspace_id = null where _id = ?", models.VolumeStateAvailable, voldId)
	if err != nil {
		return fmt.Errorf("error updating volume: %v", err)
	}
	return nil
}

// DestroyWorkspaceVolumes
//
//	Destroys all volumes associated with the passed workspace id.
//	 This should be called when a workspace is destroyed.
func (p *VolumePool) DestroyWorkspaceVolumes(workspaceId int64) error {
	// query for the volumes associated with the workspace
	res, err := p.DB.DB.Query("select * from volpool_volume where workspace_id = ?", workspaceId)
	if err != nil {
		return fmt.Errorf("error querying for workspace volumes: %v", err)
	}
	defer res.Close()

	// create slice to hold the volumes
	vols := make([]*models.VolpoolVolume, 0)

	// iterate over the results
	for res.Next() {
		// load the volume
		vol, err := models.VolpoolVolumeFromSqlNative(res)
		if err != nil {
			return fmt.Errorf("error loading volume: %v", err)
		}

		// append the volume to the slice
		vols = append(vols, vol)
	}

	_ = res.Close()

	// iterate over the volumes and destroy them
	for _, vol := range vols {
		err := p.destroyVolume(vol.ID)
		if err != nil {
			return fmt.Errorf("error destroying volume: %v", err)
		}
	}

	return nil
}

// ResolveStateDeltas
//
//	Public method to resolve state deltas which can be called from outside the pool.
//	Wraps the call in a singleflight group to prevent multiple calls from being made
//	simultaneously.
func (p *VolumePool) ResolveStateDeltas() {
	p.sflight.Do("resolveStateDeltas", func() (interface{}, error) {
		p.resolveStateDeltas()
		return nil, nil
	})
}

// resolveStateDeltas
//
//	Compares the configuration against the current state of the volume pool
//	across the cluster and determines what actions need to be taken to
//	reconcile the two.
func (p *VolumePool) resolveStateDeltas() {
	p.Logger.Debug("resolving state deltas")

	// create slices of volumes that should be provisioned and destroyed
	provisionSet := make([]*models.VolpoolVolume, 0)
	destroySet := make([]int64, 0)

	// iterate over the subpools
	for _, subpool := range p.Config.SubPools {
		p.Logger.Debugf("resolving state deltas for subpool: %d", subpool.VolumeSize)

		// get the count of available volumes in the subpool
		var availableCount int
		err := p.DB.DB.QueryRow(
			"select count(*) from volpool_volume where size = ? and state = ?",
			subpool.VolumeSize, models.VolumeStateAvailable,
		).Scan(&availableCount)
		if err != nil {
			p.Logger.Errorf("error querying for available volumes for subpool %d: %v", subpool.VolumeSize, err)
			continue
		}

		// if the count of available volumes is less than the pool size, we need to provision more
		if availableCount < subpool.PoolSize {
			// calculate the number of volumes we need to provision
			neededCount := subpool.PoolSize - availableCount

			p.Logger.Debugf("provisioning %d volumes for subpool %d", neededCount, subpool.VolumeSize)

			// create the volumes
			for i := 0; i < neededCount; i++ {
				id := p.SfNode.Generate().Int64()
				vol := models.CreateVolpoolVolume(
					id,
					subpool.VolumeSize,
					models.VolumeStateAvailable,
					fmt.Sprintf("gigo-ws-volpool-%d", id),
					subpool.StorageClass,
					nil,
				)

				provisionSet = append(provisionSet, vol)
			}
		}

		// if the count of available volumes is greater than the pool size, we need to destroy some
		if availableCount > subpool.PoolSize {
			// query for the count of volumes that are available and not in use
			res, err := p.DB.DB.Query(
				"select _id from volpool_volume where size = ? and state = ? limit ?",
				subpool.VolumeSize, models.VolumeStateAvailable, availableCount-subpool.PoolSize,
			)
			if err != nil {
				p.Logger.Errorf("error querying for available volumes for subpool %d: %v", subpool.VolumeSize, err)
				continue
			}

			// iterate over the results and add the ids to the destroy set
			for res.Next() {
				var id int64
				err := res.Scan(&id)
				if err != nil {
					p.Logger.Errorf("error scanning row for available volumes for subpool %d: %v", subpool.VolumeSize, err)
					continue
				}

				destroySet = append(destroySet, id)
			}

			_ = res.Close()

			p.Logger.Debugf("destroying %d volumes for subpool %d", len(destroySet), subpool.VolumeSize)
		}
	}

	// provision the volumes that need to be provisioned
	for _, vol := range provisionSet {
		p.Logger.Debugf("provisioning volpool volume: %d", vol.ID)
		err := p.provisionVolume(vol)
		if err != nil {
			p.Logger.Errorf("error provisioning volume %d: %v", vol.ID, err)
		}
	}

	// destroy the volumes that need to be destroyed
	for _, id := range destroySet {
		p.Logger.Debugf("destroying volpool volume: %d", id)
		err := p.destroyVolume(id)
		if err != nil {
			p.Logger.Errorf("error destroying volume %d: %v", id, err)
		}
	}
}

func (p *VolumePool) provisionVolume(vol *models.VolpoolVolume) error {
	// read template from storage
	templateBuf, err := embedFS.ReadFile("resources/storage.tf")
	if err != nil {
		return fmt.Errorf("failed to read tf template: %v", err)
	}

	// replace the volume id in the template
	template := string(templateBuf)
	template = strings.ReplaceAll(template, "<VOL_ID>", fmt.Sprintf("%d", vol.ID))

	// replace the volume size in the template
	template = strings.ReplaceAll(template, "<VOL_SIZE>", fmt.Sprintf("%dGi", vol.Size))

	// conditionally set the storage class
	sclass := ""
	if vol.StorageClass != "" {
		sclass = fmt.Sprintf("storage_class_name = \"%s\"", vol.StorageClass)
	}
	template = strings.ReplaceAll(template, "<STORAGE_CLASS>", sclass)

	// initialize environment with our current environment
	// this is really important for k8s deployment because
	// the k8s api server config is in the container's environment
	// and we won't be able to provision unless we have access to
	// that configuration within the provisioner commands
	env := os.Environ()

	// format module with terraform template
	module := &models2.TerraformModule{
		MainTF:      []byte(template),
		ModuleID:    vol.ID,
		Environment: env,
	}

	// create boolean to track failure
	failed := true

	// TODO: this seems a bit optimistic for cleanup - evaluate if this orphans resources
	// defer cleanup function to destroy resource on failure
	defer func() {
		if failed {
			_, err := p.Provisioner.Destroy(context.TODO(), module)
			if err != nil {
				p.Logger.Error(fmt.Errorf("failed to destroy volume on create cleanup: %v", err))
			}
		}
		return
	}()

	// perform apply operation
	_, err = p.Provisioner.Apply(context.TODO(), module)
	if err != nil {
		return fmt.Errorf("failed to apply configuration: %v", err)
	}

	// preserve module for later operations
	err = module.StoreModule(p.StorageEngine)
	if err != nil {
		return fmt.Errorf("failed to store module: %v", err)
	}

	// insert the volume into the database
	stmts, err := vol.ToSqlNative()
	if err != nil {
		return fmt.Errorf("failed to generate sql statements: %v", err)
	}
	tx, err := p.DB.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()
	for _, stmt := range stmts {
		_, err := tx.Exec(stmt.Statement, stmt.Values...)
		if err != nil {
			return fmt.Errorf("failed to insert volume into database: %v", err)
		}
	}
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	// mark failure as false
	failed = false

	return nil
}

func (p *VolumePool) destroyVolume(volId int64) error {
	// load module using the workspace id
	module, err := models2.LoadModule(p.StorageEngine, volId)
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
	err = models2.DeleteModule(p.StorageEngine, volId)
	if err != nil {
		return fmt.Errorf("failed to delete module from storage: %v", err)
	}

	// delete statefiles
	err = p.Provisioner.Backend.RemoveStatefile(fmt.Sprintf("states/%d", volId))
	if err != nil {
		return fmt.Errorf("failed to remove terraform statefiles: %v", err)
	}

	// delete the volume from the database
	_, err = p.DB.DB.Exec("delete from volpool_volume where _id = ?", volId)
	if err != nil {
		return fmt.Errorf("failed to delete volume from database: %v", err)
	}

	return nil
}
