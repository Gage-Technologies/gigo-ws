package config

import (
	"github.com/gage-technologies/gigo-lib/config"
	"github.com/gage-technologies/gigo-ws/models"
)

type ProvisionerBackendConfig struct {
	Type       models.ProvisionerBackendType `yaml:"provisioner_backend_type"`
	FS         config.StorageFSConfig        `yaml:"fs"`
	S3         config.StorageS3Config        `yaml:"s3"`
	InsecureS3 bool                          `yaml:"insecure_s3"`
}

type ProvisionerConfig struct {
	TerraformDir     string                   `yaml:"terraform_dir"`
	TerraformVersion string                   `yaml:"terraform_version"`
	Overwrite        bool                     `yaml:"overwrite"`
	Backend          ProvisionerBackendConfig `yaml:"backend"`
}
