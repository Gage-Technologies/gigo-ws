package config

type SubPoolConfig struct {
	VolumeSize   int    `yaml:"volume_size"`
	PoolSize     int    `yaml:"pool_size"`
	StorageClass string `yaml:"storage_class"`
}

type VolumePoolConfig struct {
	SubPools []SubPoolConfig `yaml:"sub_pools"`
}
