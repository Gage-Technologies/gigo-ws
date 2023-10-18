package backend

import (
	"github.com/gage-technologies/gigo-lib/config"
	"github.com/gage-technologies/gigo-lib/utils"
	"testing"
)

func TestNewProvisionerBackendFS(t *testing.T) {
	provisioner, err := NewProvisionerBackendFS(config.StorageFSConfig{
		Root: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	if provisioner == nil {
		t.Fatal("provisioner should not be nil")
	}
}

func TestProvisionerBackendFS_String(t *testing.T) {
	provisioner, err := NewProvisionerBackendFS(config.StorageFSConfig{
		Root: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	o := provisioner.String()

	h, err := utils.HashData([]byte(o))
	if err != nil {
		t.Fatal(err)
	}

	if h != "d3be612623fdb0a0e14c976dcc71fa391cb36c43ee679c40ff78b27b969e920a" {
		t.Fatalf("invalid hash: %s != d3be612623fdb0a0e14c976dcc71fa391cb36c43ee679c40ff78b27b969e920a\n%s", h, o)
	}
}

func TestProvisionerBackendFS_ToTerraform(t *testing.T) {
	provisioner, err := NewProvisionerBackendFS(config.StorageFSConfig{
		Root: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	o, _ := provisioner.ToTerraform("states/test-bucket")

	h, err := utils.HashData([]byte(o))
	if err != nil {
		t.Fatal(err)
	}

	if h != "6f4ca85336b825f6d6a7f47a3a4ec98ba8a3040c2d2adbecc884d0ac0b936200" {
		t.Fatalf("invalid hash: %s != 6f4ca85336b825f6d6a7f47a3a4ec98ba8a3040c2d2adbecc884d0ac0b936200\n%s", h, o)
	}
}
