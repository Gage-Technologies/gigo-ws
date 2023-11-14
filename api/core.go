package api

import (
	"context"
	"embed"
	"fmt"
	"os"
	"strings"

	"gigo-ws/config"
	"gigo-ws/models"
	"gigo-ws/provisioner"
	"gigo-ws/volpool"

	"github.com/gage-technologies/gigo-lib/logging"
	"github.com/gage-technologies/gigo-lib/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/metrics/pkg/client/clientset/versioned"
)

//go:embed resources
var embedFS embed.FS

var (
	ErrWorkspaceNotFound = fmt.Errorf("workspace not found")
)

const hostAliasesTemplate = `
    host_aliases {
      ip = "%s"
      hostnames = ["%s"]
    }
`

type templateOptions struct {
	WorkspaceID int64
	OwnerID     int64
	OwnerEmail  string
	OwnerName   string
	Disk        int
	CPU         int
	Memory      int
	Container   string
	AccessUrl   string
}

type createWorkspaceOptions struct {
	Provisioner     *provisioner.Provisioner
	Volpool         *volpool.VolumePool
	StorageEngine   storage.Storage
	TemplateOpts    templateOptions
	RegistryCaches  []config.RegistryCacheConfig
	WsHostOverrides map[string]string
	Logger          logging.Logger
}

type startWorkspaceOptions struct {
	Provisioner   *provisioner.Provisioner
	StorageEngine storage.Storage
	Logger        logging.Logger
	WorkspaceID   int64
}

type stopWorkspaceOptions struct {
	Provisioner   *provisioner.Provisioner
	StorageEngine storage.Storage
	Logger        logging.Logger
	WorkspaceID   int64
}

type destroyWorkspaceOptions struct {
	Provisioner   *provisioner.Provisioner
	Volpool       *volpool.VolumePool
	StorageEngine storage.Storage
	Logger        logging.Logger
	WorkspaceID   int64
}

type getResourceUtilOptions struct {
	KubeClient    *kubernetes.Clientset
	MetricsClient *versioned.Clientset
	WorkspaceID   int64
	OwnerID       int64
}

type ResourceUtilization struct {
	WorkspaceID int64
	OwnerID     int64
	CPU         float64
	Memory      float64
	CPULimit    int64
	MemoryLimit int64
	CPUUsage    int64
	MemoryUsage int64
}

func createWorkspace(ctx context.Context, opts createWorkspaceOptions) (*models.Agent, *provisioner.ApplyLogs, error) {
	// retrieve the current state from statefile
	state, err := provisioner.ParseStatefileForWorkspaceState(opts.Provisioner.Backend, opts.TemplateOpts.WorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse workspace state from statefile: %v", err)
	}

	// return error if the current workspace state is anything other than destroyed
	// since that would indicate that the workspace is already created
	if state != models.WorkspaceStateDestroyed {
		return nil, nil, fmt.Errorf("workspace has already been created")
	}

	// ensure that the state file is removed incase it exists
	_ = opts.Provisioner.Backend.RemoveStatefile(fmt.Sprintf("states/%d", opts.TemplateOpts.WorkspaceID))

	// attempt to retrieve a pre-existing volume - this makes provisioning faster if one exists
	vol, err := opts.Volpool.GetVolume(int64(opts.TemplateOpts.Disk), opts.TemplateOpts.WorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve volume: %v", err)
	}

	// select the template disk size if the volume is nil
	var templateBuf []byte
	if vol == nil {
		// read template from storage
		templateBuf, err = embedFS.ReadFile("resources/template_vol.tf")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read tf template: %v", err)
		}
	} else {
		// read template from storage
		templateBuf, err = embedFS.ReadFile("resources/template_novol.tf")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read tf template: %v", err)
		}

		// inject the volume name into the template
		templateBuf = []byte(strings.ReplaceAll(string(templateBuf), "<VOL_PVC_NAME>", fmt.Sprintf("%s", vol.PVCName)))
	}

	// conditionally inject host aliases into template
	if len(opts.WsHostOverrides) > 0 {
		hostAliases := ""
		for host, ip := range opts.WsHostOverrides {
			hostAliases += fmt.Sprintf(hostAliasesTemplate, ip, host)
		}
		templateBuf = []byte(strings.ReplaceAll(string(templateBuf), "<HOST_ALIASES>", hostAliases))
	}

	// update the container with registry caching if it is configured
	opts.TemplateOpts.Container = handleRegistryCaches(opts.TemplateOpts.Container, opts.RegistryCaches)

	// format module with terraform template
	module := &models.TerraformModule{
		MainTF:      templateBuf,
		ModuleID:    opts.TemplateOpts.WorkspaceID,
		Environment: prepEnvironmentForCreation(opts.TemplateOpts),
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
			_, err := opts.Provisioner.Destroy(context.TODO(), module)
			if err != nil {
				opts.Logger.Error(fmt.Errorf("failed to destroy workspace on create cleanup: %v", err))
			}

			// release the volume if it exists
			if vol != nil {
				err = opts.Volpool.ReleaseVolume(vol.ID)
				if err != nil {
					opts.Logger.Error(fmt.Errorf("failed to release volume on create cleanup: %v", err))
				}
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
	logs, err := opts.Provisioner.Apply(ctx, module)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply configuration: %v", err)
	}

	// retrieve agent from statefile
	agent, err := provisioner.ParseStatefileForAgent(opts.Provisioner.Backend, opts.TemplateOpts.WorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse agent from statefile: %v", err)
	}

	// preserve module for later operations
	err = module.StoreModule(opts.StorageEngine)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to store module: %v", err)
	}

	// mark operation as a success to prevent cleanup
	failed = false

	// return apply logs
	return agent, logs, nil
}

func startWorkspace(ctx context.Context, opts startWorkspaceOptions) (*models.Agent, *provisioner.ApplyLogs, error) {
	// retrieve the current state from statefile
	state, err := provisioner.ParseStatefileForWorkspaceState(opts.Provisioner.Backend, opts.WorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse workspace state from statefile: %v", err)
	}

	// handle a destroyed workspace by returning an error
	// we cannot recover from removing the pvc
	if state == models.WorkspaceStateDestroyed {
		// ensure that the state file is removed incase it exists
		_ = opts.Provisioner.Backend.RemoveStatefile(fmt.Sprintf("states/%d", opts.WorkspaceID))
		return nil, nil, ErrWorkspaceNotFound
	}

	// create dummy logs incase we don't do anything
	logs := &provisioner.ApplyLogs{
		StdOut: make([]map[string]interface{}, 0),
		StdErr: make([]map[string]interface{}, 0),
	}

	// only perform the operation if we are stopped
	if state == models.WorkspaceStateStopped {
		// load module using the workspace id
		module, err := models.LoadModule(opts.StorageEngine, opts.WorkspaceID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load module: %v", err)
		}
		if module == nil {
			return nil, nil, ErrWorkspaceNotFound
		}

		// append start transition to module environment
		module.Environment = append(module.Environment, "GIGO_WORKSPACE_TRANSITION=start")

		// TODO: this seems a bit optimistic for cleanup - evaluate if this orphans resources
		// defer cleanup function to destroy resource on failure
		defer func() {
			// clean up the temporary module on fs
			err := os.RemoveAll(module.LocalPath)
			if err != nil {
				opts.Logger.Error(fmt.Errorf("failed to clean up temporary module on create cleanup: %v", err))
			}
			return
		}()

		// perform apply operation
		logs, err = opts.Provisioner.Apply(ctx, module)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply configuration: %v", err)
		}

		// TODO: think long and hard about what a cleanup operation looks like for this
		// do we stop the workspace, make a second attempt???
	}

	// retrieve agent from statefile
	agent, err := provisioner.ParseStatefileForAgent(opts.Provisioner.Backend, opts.WorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse agent from statefile: %v", err)
	}

	// return apply logs
	return agent, logs, nil
}

func stopWorkspace(ctx context.Context, opts stopWorkspaceOptions) (*models.Agent, *provisioner.ApplyLogs, error) {
	// retrieve the current state from statefile
	state, err := provisioner.ParseStatefileForWorkspaceState(opts.Provisioner.Backend, opts.WorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse workspace state from statefile: %v", err)
	}

	// handle a destroyed workspace by returning an error
	// we cannot recover from removing the pvc
	if state == models.WorkspaceStateDestroyed {
		// ensure that the state file is removed incase it exists
		_ = opts.Provisioner.Backend.RemoveStatefile(fmt.Sprintf("states/%d", opts.WorkspaceID))
		return nil, nil, ErrWorkspaceNotFound
	}

	// create dummy logs incase we don't do anything
	logs := &provisioner.ApplyLogs{
		StdOut: make([]map[string]interface{}, 0),
		StdErr: make([]map[string]interface{}, 0),
	}

	// only perform the operation if we are active
	if state == models.WorkspaceStateActive {
		// load module using the workspace id
		module, err := models.LoadModule(opts.StorageEngine, opts.WorkspaceID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load module: %v", err)
		}
		if module == nil {
			return nil, nil, ErrWorkspaceNotFound
		}

		// append stop transition to module environment
		module.Environment = append(module.Environment, "GIGO_WORKSPACE_TRANSITION=stop")

		// TODO: this seems a bit optimistic for cleanup - evaluate if this orphans resources
		// defer cleanup function to destroy resource on failure
		defer func() {
			// clean up the temporary module on fs
			err := os.RemoveAll(module.LocalPath)
			if err != nil {
				opts.Logger.Error(fmt.Errorf("failed to clean up temporary module on create cleanup: %v", err))
			}
			return
		}()

		// perform apply operation
		logs, err = opts.Provisioner.Apply(ctx, module)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply configuration: %v", err)
		}

		// TODO: think long and hard about what a cleanup operation looks like for this
		// do we restart the workspace, make a second attempt???
	}

	// retrieve agent from statefile
	agent, err := provisioner.ParseStatefileForAgent(opts.Provisioner.Backend, opts.WorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse agent from statefile: %v", err)
	}

	// return apply logs
	return agent, logs, nil
}

func destroyWorkspace(ctx context.Context, opts destroyWorkspaceOptions) (*provisioner.DestroyLogs, error) {
	// retrieve the current state from statefile
	state, err := provisioner.ParseStatefileForWorkspaceState(opts.Provisioner.Backend, opts.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workspace state from statefile: %v", err)
	}

	// handle a destroyed workspace by returning a no-op
	if state == models.WorkspaceStateDestroyed {
		// ensure that the state file is removed incase it exists
		_ = opts.Provisioner.Backend.RemoveStatefile(fmt.Sprintf("states/%d", opts.WorkspaceID))
		return &provisioner.DestroyLogs{
			StdOut: make([]map[string]interface{}, 0),
			StdErr: make([]map[string]interface{}, 0),
		}, ErrWorkspaceNotFound
	}

	// load module using the workspace id
	module, err := models.LoadModule(opts.StorageEngine, opts.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to load module: %v", err)
	}
	if module == nil {
		return nil, ErrWorkspaceNotFound
	}

	// append stop transition to module environment
	module.Environment = append(module.Environment, "GIGO_WORKSPACE_TRANSITION=destroy")

	// TODO: this seems a bit optimistic for cleanup - evaluate if this orphans resources
	// defer cleanup function to destroy resource on failure
	defer func() {
		// clean up the temporary module on fs
		err := os.RemoveAll(module.LocalPath)
		if err != nil {
			opts.Logger.Error(fmt.Errorf("failed to clean up temporary module on create cleanup: %v", err))
		}
		return
	}()

	// perform apply operation
	logs, err := opts.Provisioner.Destroy(ctx, module)
	if err != nil {
		return nil, fmt.Errorf("failed to destroy configuration: %v", err)
	}

	// clean up persistent state of the workspace
	err = opts.Volpool.DestroyWorkspaceVolumes(opts.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to destroy workspace volumes: %v", err)
	}

	// delete the module from storage
	err = models.DeleteModule(opts.StorageEngine, opts.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete module from storage: %v", err)
	}

	// delete statefiles
	err = opts.Provisioner.Backend.RemoveStatefile(fmt.Sprintf("states/%d", opts.WorkspaceID))
	if err != nil {
		return nil, fmt.Errorf("failed to remove terraform statefiles: %v", err)
	}

	return logs, nil
}

func getResourceUtil(ctx context.Context, opts getResourceUtilOptions) (*ResourceUtilization, error) {
	// retrieve the pods specification
	pod, err := opts.KubeClient.
		CoreV1().
		Pods("gigo-ws-prov-plane").
		Get(
			context.TODO(),
			fmt.Sprintf("gigo-ws-%d-%d", opts.OwnerID, opts.WorkspaceID),
			metav1.GetOptions{},
		)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pod spec: %w", err)
	}

	// retrieve the first container
	containerSpec := pod.Spec.Containers[0]

	// rertieve the allocations
	allocatedCPU := containerSpec.Resources.Limits.Cpu().MilliValue()
	allocatedMemory := containerSpec.Resources.Limits.Memory().MilliValue()

	// retrieve the pods utilization
	podMetrics, err := opts.MetricsClient.
		MetricsV1beta1().
		PodMetricses("gigo-ws-prov-plane").
		Get(
			ctx,
			fmt.Sprintf("gigo-ws-%d-%d", opts.OwnerID, opts.WorkspaceID),
			metav1.GetOptions{},
		)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}

	// create a new resource utilization object
	util := ResourceUtilization{
		WorkspaceID: opts.WorkspaceID,
		OwnerID:     opts.OwnerID,
		CPULimit:    allocatedCPU,
		MemoryLimit: allocatedMemory,
	}

	// calculate the percentage of the utilization
	for _, c := range podMetrics.Containers {
		util.CPUUsage = c.Usage.Cpu().MilliValue()
		util.MemoryUsage = c.Usage.Memory().MilliValue()
		util.CPU = float64(util.CPUUsage) / float64(allocatedCPU)
		util.Memory = float64(util.MemoryUsage) / float64(allocatedMemory)
	}

	return &util, nil
}

func prepEnvironmentForCreation(opts templateOptions) []string {
	// initialize environment with our current environment
	// this is really important for k8s deployment because
	// the k8s api server config is in the container's environment
	// and we won't be able to provision unless we have access to
	// that configuration within the provisioner commands
	env := os.Environ()

	// format parameters into environment variables so that
	// terraform can compute the provisioning state
	env = append(env,
		fmt.Sprintf("GIGO_WORKSPACE_OWNER=%s", opts.OwnerName),
		fmt.Sprintf("GIGO_WORKSPACE_OWNER_EMAIL=%s", opts.OwnerEmail),
		fmt.Sprintf("GIGO_WORKSPACE_OWNER_ID=%d", opts.OwnerID),
		fmt.Sprintf("GIGO_WORKSPACE_DISK=%dGi", opts.Disk),
		fmt.Sprintf("GIGO_WORKSPACE_CPU=%d", opts.CPU),
		fmt.Sprintf("GIGO_WORKSPACE_MEM=%dG", opts.Memory),
		fmt.Sprintf("GIGO_WORKSPACE_CONTAINER=%s", opts.Container),
		fmt.Sprintf("GIGO_AGENT_URL=%s", opts.AccessUrl),
		fmt.Sprintf("GIGO_WORKSPACE_ID=%d", opts.WorkspaceID),
		"GIGO_WORKSPACE_TRANSITION=start",
	)

	// add agent scripts to the environments
	env = append(env, AgentScriptEnv()...)

	return env
}

// handleRegistryCaches
//
//	Checks if the container is from any of the source registries
//	and if so, replaces the host with container registry cache in the container
//	name. If there is a cache configured for docker.io and the container
//	contains no host then the container name is assumed to be from docker.io
func handleRegistryCaches(containerName string, caches []config.RegistryCacheConfig) string {
	// create a variable to hold the docker.io cache if it exists
	var dockerCache config.RegistryCacheConfig

	// iterate over the registry caches
	for _, cache := range caches {
		// if the container name contains the registry host
		if strings.HasPrefix(containerName, cache.Source) {
			// replace the registry host with the cache host
			return strings.Replace(containerName, cache.Source, cache.Cache, 1)
		}

		// save the docker cache if it exists in case the container has no host prefix
		if cache.Source == "docker.io" {
			// set the docker cache
			dockerCache = cache
		}
	}

	// if the container name has no host prefix and the docker cache exists
	// then we assume the container is from docker.io and prepend the cache
	if dockerCache.Source == "docker.io" && strings.Count(containerName, "/") <= 1 {
		return fmt.Sprintf("%s/%s", dockerCache.Cache, containerName)
	}

	// return the container name if no cache was found
	return containerName
}
