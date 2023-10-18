package provisioner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gigo-ws/config"
	"gigo-ws/models"
	utils2 "gigo-ws/utils"

	config2 "github.com/gage-technologies/gigo-lib/config"
	"github.com/gage-technologies/gigo-lib/logging"
	"github.com/gage-technologies/gigo-lib/utils"
)

const testTerraformMain = `terraform {
  <BACKEND_PROVIDER>

  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
      version = ">= 2.13.0"
    }
  }
}

provider "docker" {
  host    = "unix:///var/run/docker.sock"
}

resource "docker_image" "nginx" {
  name         = "nginx:latest"
  keep_locally = false
}

resource "docker_container" "nginx" {
  image = docker_image.nginx.name
  name  = "<CONTAINER_NAME>"
  ports {
    internal = 80
    external = 8000
  }
}`

func TestNewProvisioner(t *testing.T) {
	logger, err := logging.CreateBasicLogger(logging.NewDefaultBasicLoggerOptions("/tmp/gigo-ws-new-provisioner-test.log"))
	if err != nil {
		t.Fatal(err)
	}

	p, err := NewProvisioner(config.ProvisionerConfig{
		TerraformDir:     "/tmp/gigo-ws-new-tf-bin",
		TerraformVersion: "1.3.7",
		Overwrite:        true,
		Backend: config.ProvisionerBackendConfig{
			Type: models.ProvisionerBackendFS,
			FS: config2.StorageFSConfig{
				Root: "/tmp/gigo-ws-new-Backend",
			},
		},
	}, logger)
	if err != nil {
		t.Fatal(err)
	}

	if p == nil {
		t.Fatalf("provisioner should not be nil")
	}
}

func TestNewProvisioner_prepModule(t *testing.T) {
	logger, err := logging.CreateBasicLogger(logging.NewDefaultBasicLoggerOptions("/tmp/gigo-ws-prep-provisioner-test.log"))
	if err != nil {
		t.Fatal(err)
	}

	p, err := NewProvisioner(config.ProvisionerConfig{
		TerraformDir:     "/tmp/gigo-ws-prep-tf-bin",
		TerraformVersion: "1.3.7",
		Overwrite:        true,
		Backend: config.ProvisionerBackendConfig{
			Type: models.ProvisionerBackendFS,
			FS: config2.StorageFSConfig{
				Root: "/tmp/gigo-ws-provisioner-Backend",
			},
		},
	}, logger)
	if err != nil {
		t.Fatal(err)
	}

	module := &models.TerraformModule{
		MainTF:   []byte(testTerraformMain),
		ModuleID: 420,
	}

	err = p.prepModule(context.Background(), module)
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(module.LocalPath)

	if bytes.Contains(module.MainTF, []byte("<BACKEND_PROVIDER>")) {
		t.Fatalf("Backend provider template should not be present")
	}

	exists, err := utils.PathExists(filepath.Join(module.LocalPath, ".terraform"))
	if err != nil {
		t.Fatal(err)
	}

	if !exists {
		t.Fatalf("module was not initialized")
	}

	err = p.prepModule(context.Background(), module)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewProvisioner_Validate(t *testing.T) {
	logger, err := logging.CreateBasicLogger(logging.NewDefaultBasicLoggerOptions("/tmp/gigo-ws-prep-provisioner-test.log"))
	if err != nil {
		t.Fatal(err)
	}

	p, err := NewProvisioner(config.ProvisionerConfig{
		TerraformDir:     "/tmp/gigo-ws-prep-tf-bin",
		TerraformVersion: "1.3.7",
		Overwrite:        true,
		Backend: config.ProvisionerBackendConfig{
			Type: models.ProvisionerBackendFS,
			FS: config2.StorageFSConfig{
				Root: "/tmp/gigo-ws-provisioner-Backend",
			},
		},
	}, logger)
	if err != nil {
		t.Fatal(err)
	}

	module := &models.TerraformModule{
		MainTF:   []byte(testTerraformMain),
		ModuleID: 420,
	}

	validationResponse, err := p.Validate(context.Background(), module)
	if err != nil {
		t.Fatal(err)
	}

	if validationResponse != nil {
		t.Fatalf("validation response should be nil")
	}

	defer os.RemoveAll(module.LocalPath)

	if bytes.Contains(module.MainTF, []byte("<BACKEND_PROVIDER>")) {
		t.Fatalf("Backend provider template should not be present")
	}

	exists, err := utils.PathExists(filepath.Join(module.LocalPath, ".terraform"))
	if err != nil {
		t.Fatal(err)
	}

	if !exists {
		t.Fatalf("module was not initialized")
	}

	module = &models.TerraformModule{
		MainTF:   []byte(strings.ReplaceAll(testTerraformMain, "required_providers", "")),
		ModuleID: 420,
	}

	validationResponse, err = p.Validate(context.Background(), module)
	if err == nil {
		t.Fatalf("validation response error should not be nil")
	}

	_ = os.RemoveAll(module.LocalPath)
}

func TestNewProvisioner_Apply(t *testing.T) {
	logger, err := logging.CreateBasicLogger(logging.NewDefaultBasicLoggerOptions("/tmp/gigo-ws-prep-provisioner-test.log"))
	if err != nil {
		t.Fatal(err)
	}

	p, err := NewProvisioner(config.ProvisionerConfig{
		TerraformDir:     "/tmp/gigo-ws-apply-tf-bin",
		TerraformVersion: "1.3.7",
		Overwrite:        true,
		Backend: config.ProvisionerBackendConfig{
			Type: models.ProvisionerBackendFS,
			FS: config2.StorageFSConfig{
				Root: "/tmp/gigo-ws-provisioner-Backend",
			},
		},
	}, logger)
	if err != nil {
		t.Fatal(err)
	}

	module := &models.TerraformModule{
		MainTF:   []byte(strings.ReplaceAll(testTerraformMain, "<CONTAINER_NAME>", "gigo-ws-apply-tf")),
		ModuleID: 420,
	}

	defer func() {
		_, _ = utils2.ExecuteCommand(
			context.TODO(), nil, "",
			"sh", "-c",
			fmt.Sprintf(
				"cd %s && %s destroy -json -auto-approve -no-color",
				module.LocalPath, p.terraformPath,
			),
		)
		_ = os.RemoveAll(module.LocalPath)
	}()

	applyLogs, err := p.Apply(context.Background(), module)
	if err != nil {
		t.Fatal(err)
	}

	if applyLogs == nil {
		t.Fatalf("apply result should not be nil")
	}

	if len(applyLogs.StdOut) != 20 {
		t.Fatalf("stdout length should be 20, got %d", len(applyLogs.StdOut))
	}

	if len(applyLogs.StdErr) != 0 {
		t.Fatalf("stderr length should be 0, got %d", len(applyLogs.StdErr))
	}
}

func TestNewProvisioner_Destroy(t *testing.T) {
	logger, err := logging.CreateBasicLogger(logging.NewDefaultBasicLoggerOptions("/tmp/gigo-ws-prep-provisioner-test.log"))
	if err != nil {
		t.Fatal(err)
	}

	p, err := NewProvisioner(config.ProvisionerConfig{
		TerraformDir:     "/tmp/gigo-ws-destroy-tf-bin",
		TerraformVersion: "1.3.7",
		Overwrite:        true,
		Backend: config.ProvisionerBackendConfig{
			Type: models.ProvisionerBackendFS,
			FS: config2.StorageFSConfig{
				Root: "/tmp/gigo-ws-provisioner-Backend",
			},
		},
	}, logger)
	if err != nil {
		t.Fatal(err)
	}

	module1 := &models.TerraformModule{
		MainTF:   []byte(strings.ReplaceAll(testTerraformMain, "<CONTAINER_NAME>", "gigo-ws-destroy-tf")),
		ModuleID: 42069,
	}
	module2 := &models.TerraformModule{
		MainTF:   []byte(strings.ReplaceAll(testTerraformMain, "<CONTAINER_NAME>", "gigo-ws-destroy-tf")),
		ModuleID: 42069,
	}

	defer func() {
		_, _ = utils2.ExecuteCommand(
			context.TODO(), nil, "",
			"sh", "-c",
			fmt.Sprintf(
				"cd %s && %s destroy -json -auto-approve -no-color",
				module1.LocalPath, p.terraformPath,
			),
		)
		_, _ = utils2.ExecuteCommand(
			context.TODO(), nil, "",
			"sh", "-c",
			fmt.Sprintf(
				"cd %s && %s destroy -json -auto-approve -no-color",
				module2.LocalPath, p.terraformPath,
			),
		)
		_ = os.RemoveAll(module1.LocalPath)
		_ = os.RemoveAll(module2.LocalPath)
	}()

	_, err = p.Apply(context.Background(), module1)
	if err != nil {
		t.Fatal(err)
	}

	_ = os.RemoveAll(module1.LocalPath)

	destroyLogs, err := p.Destroy(context.Background(), module2)
	if err != nil {
		t.Fatal(err)
	}

	if destroyLogs == nil {
		t.Fatalf("apply result should not be nil")
	}

	if len(destroyLogs.StdOut) != 26 {
		t.Fatalf("stdout length should be 26, got %d", len(destroyLogs.StdOut))
	}

	if len(destroyLogs.StdErr) != 0 {
		t.Fatalf("stderr length should be 0, got %d", len(destroyLogs.StdErr))
	}
}
