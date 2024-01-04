package config

type WsSubPoolConfig struct {
	Container    string `yaml:"container"`
	Memory       int64  `yaml:"memory"`
	CPU          int64  `yaml:"cpu"`
	VolumeSize   int    `yaml:"volume_size"`
	PoolSize     int    `yaml:"pool_size"`
	StorageClass string `yaml:"storage_class"`
}

type WsPoolConfig struct {
	SubPools   []WsSubPoolConfig `yaml:"ws_sub_pools"`
	OwnerName  string            `yaml:"owner_name"`
	OwnerEmail string            `yaml:"owner_email"`
	OwnerID    int64             `yaml:"owner_id"`
	AccessUrl  string            `yaml:"access_url"`
}
