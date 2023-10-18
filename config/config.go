package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/gage-technologies/gigo-lib/config"
	"gopkg.in/yaml.v3"
)

type LoggerConfig struct {
	ESConfig config.ElasticConfig `yaml:"es"`
}

type RegistryCacheConfig struct {
	Source string `yaml:"source"`
	Cache  string `yaml:"cache"`
}

type Config struct {
	TitaniumConfig   config.TitaniumConfig `yaml:"ti_config"`
	Cluster          bool                  `yaml:"cluster"`
	Provisioner      ProvisionerConfig     `yaml:"provisioner"`
	ModuleStorage    config.StorageConfig  `yaml:"module_storage"`
	Server           ServerConfig          `yaml:"server"`
	EtcdConfig       config.EtcdConfig     `yaml:"etcd_config"`
	RegistryCaches   []RegistryCacheConfig `yaml:"registry_caches"`
	Logger           LoggerConfig          `yaml:"logger"`
	WsHostOverrides  map[string]string     `yaml:"ws_host_overrides"`
	VolumePoolConfig VolumePoolConfig      `yaml:"volume_pool"`
}

func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("failed to open config file: %v", err))
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("failed to read config file contents: %v", err))
	}

	var cfg Config
	err = yaml.Unmarshal(b, &cfg)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("failed to decode config file: %v", err))
	}

	err = f.Close()
	if err != nil {
		return nil, errors.New(fmt.Sprintf("failed to decode config file: %v", err))
	}

	return &cfg, nil
}
