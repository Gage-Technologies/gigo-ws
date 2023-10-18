package backend

import (
	"github.com/gage-technologies/gigo-lib/config"
	"github.com/gage-technologies/gigo-lib/utils"
	"testing"
)

func TestNewProvisionerBackendS3(t *testing.T) {
	provisioner, err := NewProvisionerBackendS3(config.StorageS3Config{
		Bucket:    "test",
		Region:    "us-west-2",
		Endpoint:  "127.0.0.1:9000",
		SecretKey: "test",
		AccessKey: "test",
	}, true)
	if err != nil {
		t.Fatal(err)
	}

	if provisioner == nil {
		t.Fatal("provisioner should not be nil")
	}
}

func TestProvisionerBackendS3_String(t *testing.T) {
	provisioner, err := NewProvisionerBackendS3(config.StorageS3Config{
		Bucket:    "test",
		Region:    "us-west-2",
		Endpoint:  "127.0.0.1:9000",
		SecretKey: "test",
		AccessKey: "test",
	}, true)
	if err != nil {
		t.Fatal(err)
	}

	o := provisioner.String()

	h, err := utils.HashData([]byte(o))
	if err != nil {
		t.Fatal(err)
	}

	if h != "b38e56dde247a3b9afe3c327895e8c46c8e53e0735dacdd629057b5fa1b4ab92" {
		t.Fatalf("invalid hash: %s != b38e56dde247a3b9afe3c327895e8c46c8e53e0735dacdd629057b5fa1b4ab92\n%s", h, o)
	}
}

func TestProvisionerBackendS3_ToTerraform(t *testing.T) {
	provisioner, err := NewProvisionerBackendS3(config.StorageS3Config{
		Bucket:    "test",
		Region:    "us-west-2",
		Endpoint:  "127.0.0.1:9000",
		SecretKey: "test",
		AccessKey: "test",
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	o, _ := provisioner.ToTerraform("states/test-bucket")

	h, err := utils.HashData([]byte(o))
	if err != nil {
		t.Fatal(err)
	}

	if h != "6a3f9f104641f646b6bb1f8f08cb14f22d29086970f03c9276ef57064d9f8585" {
		t.Fatalf("invalid hash: %s != 6a3f9f104641f646b6bb1f8f08cb14f22d29086970f03c9276ef57064d9f8585\n%s", h, o)
	}
}
