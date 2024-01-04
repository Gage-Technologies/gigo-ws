package main

import (
	"context"
	"flag"
	"fmt"
	"gigo-ws/ws_pool"
	"log"
	"math"
	"os"
	"sync"
	"syscall"
	"time"

	"gigo-ws/api"
	"gigo-ws/config"
	"gigo-ws/provisioner"
	"gigo-ws/volpool"

	"github.com/bwmarrin/snowflake"
	"github.com/gage-technologies/gigo-lib/cluster"
	config2 "github.com/gage-technologies/gigo-lib/config"
	ti "github.com/gage-technologies/gigo-lib/db"
	"github.com/gage-technologies/gigo-lib/logging"
	"github.com/gage-technologies/gigo-lib/storage"
	"github.com/syossan27/tebata"
	etcd "go.etcd.io/etcd/client/v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/metrics/pkg/client/clientset/versioned"
)

// TODO: add graceful shutdown

var (
	lock        = &sync.Mutex{}
	interrupted = false
)

func shutdown(server *api.ProvisionerApiServer, clusterNode cluster.Node, clusterCancel context.CancelFunc,
	logger logging.Logger) {
	// we lock here so we can prevent the main thread from exiting
	// before we finish the graceful shutdown
	lock.Lock()
	defer lock.Unlock()

	// mark as interrupted
	interrupted = true

	// log that we received a shutdown request
	logger.Info("received termination - shutting down gracefully")

	// close server gracefully
	logger.Info("closing server")
	err := server.Close()
	if err != nil {
		logger.Errorf("failed to close server gracefully: %v", err)
	}

	// close cluster node
	logger.Info("closing cluster node")
	clusterNode.Stop()
	err = clusterNode.Close()
	if err != nil {
		logger.Errorf("failed to close cluster node gracefully: %v", err)
	}
	clusterCancel()

	// flush logger so we get any last logs
	logger.Flush()
}

func main() {
	// set timezone to US Central
	err := os.Setenv("TZ", "America/Chicago")
	if err != nil {
		panic(err)
	}

	configPath := flag.String("config", "/config.yml", "Path to the configuration file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatal("failed to load config ", err)
	}

	fmt.Println("Registry Caches: ", cfg.RegistryCaches)

	// Use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		log.Fatal("failed to build kubernetes config: ", err)
	}

	// Create a Kubernetes client
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal("failed to create kubernetes client: ", err)
	}

	// Create a Metrics client
	metricsClient, err := versioned.NewForConfig(config)
	if err != nil {
		log.Fatal("failed to create metrics client: ", err)
	}

	// create a snowflake node to create node ids for the provisioners
	// we reserve node number 1021 for provisioners so that there are not id
	// collisions
	snowflakeNode, err := snowflake.NewNode(1021)
	if err != nil {
		log.Fatalf("failed to create snowflake node: %v", err)
	}

	// create node id
	nodeId := snowflakeNode.Generate()

	fmt.Println("Node Id: ", nodeId.Int64())

	// initialize logger
	clusterLogger, err := logging.CreateESLogger(
		cfg.Logger.ESConfig.ESNodes,
		cfg.Logger.ESConfig.Username,
		cfg.Logger.ESConfig.ESPass,
		cfg.Logger.ESConfig.Index+"-cluster",
		nodeId.String(),
	)
	if err != nil {
		log.Fatal("failed to create logger ", err)
	}

	// initialize logger
	logger, err := logging.CreateESLogger(
		cfg.Logger.ESConfig.ESNodes,
		cfg.Logger.ESConfig.Username,
		cfg.Logger.ESConfig.ESPass,
		cfg.Logger.ESConfig.Index,
		nodeId.String(),
	)
	if err != nil {
		log.Fatal("failed to create logger ", err)
	}

	// create variable for storage engine
	var storageEngine storage.Storage

	// initialize storage engine
	switch cfg.ModuleStorage.Engine {
	case config2.StorageEngineS3:
		storageEngine, err = storage.CreateMinioObjectStorage(cfg.ModuleStorage.S3)
		if err != nil {
			log.Fatalf("failed to create s3 object storage engine: %v", err)
		}
	case config2.StorageEngineFS:
		storageEngine, err = storage.CreateFileSystemStorage(cfg.ModuleStorage.FS.Root)
		if err != nil {
			log.Fatalf("failed to create fs storage engine: %v", err)
		}
	default:
		log.Fatalf("invalid storage engine: %s", cfg.ModuleStorage.Engine)
	}

	fmt.Println("Creating ti database")

	tiDB, err := ti.CreateDatabase(cfg.TitaniumConfig.TitaniumHost, cfg.TitaniumConfig.TitaniumPort, "mysql", cfg.TitaniumConfig.TitaniumUser,
		cfg.TitaniumConfig.TitaniumPassword, cfg.TitaniumConfig.TitaniumName)
	if err != nil {
		log.Fatal("failed to create titanium database: ", err)
	}

	// initialize provisioner
	prov, err := provisioner.NewProvisioner(cfg.Provisioner, logger)
	if err != nil {
		log.Fatalf("failed to create provisioner: %v", err)
	}

	logger.Info("initializing volume pool")

	// create a new volume pool
	vpool := volpool.NewVolumePool(volpool.VolumePoolParams{
		DB:            tiDB,
		Provisioner:   prov,
		StorageEngine: storageEngine,
		SfNode:        snowflakeNode,
		Logger:        logger,
		Config:        cfg.VolumePoolConfig,
	})

	wsPool := ws_pool.NewWorkspacePool(ws_pool.WorkspacePoolParams{
		DB:              tiDB,
		Provisioner:     prov,
		StorageEngine:   storageEngine,
		SfNode:          snowflakeNode,
		Logger:          logger,
		Config:          cfg.WorkspacePoolConfig,
		WsHostOverrides: cfg.WsHostOverrides,
		RegistryCaches:  cfg.RegistryCaches,
	})

	// create context for cluster
	clusterCtx, clusterCancel := context.WithCancel(context.Background())

	// create cluster node
	var clusterNode cluster.Node
	if !cfg.Cluster {
		clusterNode = cluster.NewStandaloneNode(
			clusterCtx,
			nodeId.Int64(),
			// we assume that the node ip will always be set at this
			// env var - this is really designed to be operated on k8s
			// but could theoretically be set manually if deployed by hand
			os.Getenv("GIGO_POD_IP"),
			// we don't utilize any coordination amongst the cluster nodes other
			// that ephemeral storage bound to the nodes lease
			func(ctx context.Context) error {
				return nil
			},
			func(ctx context.Context) error {
				return nil
			},
			// we never tick so set to a ridiculous value
			math.MaxInt64,
			clusterLogger,
		)
	} else {
		clusterNode, err = cluster.NewClusterNode(cluster.ClusterNodeOptions{
			clusterCtx,
			nodeId.Int64(),
			// we assume that the node ip will always be set at this
			// env var - this is really designed to be operated on k8s
			// but could theoretically be set manually if deployed by hand
			os.Getenv("GIGO_POD_IP"),
			// we have a 10 second timeout on the node such that if the node
			// dies or hangs for more than 10 seconds we will consider the node
			// dead and will elect a new leader forcing the node to exit the
			// cluster and rejoin with a new role
			time.Second * 10,
			"gigo-ws",
			etcd.Config{
				Endpoints: cfg.EtcdConfig.Hosts,
				Username:  cfg.EtcdConfig.Username,
				Password:  cfg.EtcdConfig.Password,
			},
			func(ctx context.Context) error {
				go func() {
					vpool.ResolveStateDeltas()
					wsPool.ResolveStateDeltas()
				}()
				return nil
			},
			func(ctx context.Context) error {
				return nil
			},
			time.Second * 5,
			clusterLogger,
		})
		if err != nil {
			log.Fatalf("failed to create cluster node: %v", err)
		}
	}

	// start the cluster node
	clusterNode.Start()

	// create server
	server, err := api.NewProvisionerApiServer(api.ProvisionerApiServerOptions{
		ID:              nodeId.Int64(),
		ClusterNode:     clusterNode,
		Provisioner:     prov,
		Volpool:         vpool,
		WsPool:          wsPool,
		KubeClient:      kubeClient,
		MetricsClient:   metricsClient,
		StorageEngine:   storageEngine,
		SnowflakeNode:   snowflakeNode,
		Host:            cfg.Server.Host,
		Port:            cfg.Server.Port,
		RegistryCaches:  cfg.RegistryCaches,
		WsHostOverrides: cfg.WsHostOverrides,
		Logger:          logger,
	})
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	// register shutdown handler for all potential interrupt signals
	interrupt := tebata.New(syscall.SIGINT)
	err = interrupt.Reserve(shutdown, server, clusterNode, clusterCancel, logger)
	if err != nil {
		log.Fatal("failed to created interrupt handler: ", err)
	}

	term := tebata.New(syscall.SIGTERM)
	err = term.Reserve(shutdown, server, clusterNode, clusterCancel, logger)
	if err != nil {
		log.Fatal("failed to created term handler: ", err)
	}

	kill := tebata.New(syscall.SIGKILL)
	err = kill.Reserve(shutdown, server, clusterNode, clusterCancel, logger)
	if err != nil {
		log.Fatal("failed to created kill handler: ", err)
	}

	// start server
	err = server.Serve()

	// acquire lock so we can be sure that any graceful shutdown has completed
	lock.Lock()
	defer lock.Unlock()

	// only log the error if we didn't gracefully shutdown
	if err != nil && !interrupted {
		logger.Errorf("server failed unexpectedly: %v", err)
		// gracefully shutdown
		shutdown(server, clusterNode, clusterCancel, logger)
	}
}
