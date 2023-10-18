package models

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/gage-technologies/gigo-lib/storage"
	"golang.org/x/crypto/sha3"
)

const testTerraformMain = `terraform {
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
  name  = "tutorial"
  ports {
    internal = 80
    external = 8000
  }
}`

func TestTerraformModule_WriteModule(t *testing.T) {
	module := TerraformModule{
		MainTF:   []byte(testTerraformMain),
		ModuleID: 420,
	}

	err := module.WriteTemporaryCopy()
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(module.LocalPath)

	files, err := os.ReadDir(module.LocalPath)
	if err != nil {
		t.Fatal(err)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	hasher := sha3.New256()
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(module.LocalPath, f.Name()))
		if err != nil {
			t.Fatal(err)
		}
		_, err = hasher.Write(b)
		if err != nil {
			t.Fatal(err)
		}
	}
	h := hex.EncodeToString(hasher.Sum(nil))

	if h != "8bf1708080c3b034878fcf3c6581d8eb12ed926d0a8362cb769254199def356d" {
		t.Fatalf("incorrect module directory - hash %s != 8bf1708080c3b034878fcf3c6581d8eb12ed926d0a8362cb769254199def356d", h)
	}

	err = module.WriteTemporaryCopy()
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(module.LocalPath, "main.tf"), []byte("freak out"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = module.WriteTemporaryCopy()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTerraformModule_ModuleIO(t *testing.T) {
	module := &TerraformModule{
		MainTF:    []byte(testTerraformMain),
		ModuleID:  420,
		LocalPath: "/test1",
		Validated: true,
		Environment: []string{
			"FOO=bar",
			"BAR=baz",
		},
	}

	storageEngine, err := storage.CreateFileSystemStorage("/tmp/gigo-ws-tf-mod-io-test")
	if err != nil {
		t.Fatal(err)
	}

	err = module.StoreModule(storageEngine)
	if err != nil {
		t.Fatal(err)
	}

	mod, err := LoadModule(storageEngine, 420)
	if err != nil {
		t.Fatal(err)
	}

	if mod.LocalPath != "" {
		t.Fatalf("expected empty module path, got %s", mod.LocalPath)
	}

	module.LocalPath = ""

	if !reflect.DeepEqual(*module, *mod) {
		t.Fatalf("expected %+v\ngot      %+v", *module, *mod)
	}

	err = DeleteModule(storageEngine, 420)
	if err != nil {
		t.Fatal(err)
	}

	exists, _, err := storageEngine.Exists(fmt.Sprintf("modules/420"))
	if err != nil {
		t.Fatal(err)
	}

	if exists {
		t.Fatal("expected module to be deleted")
	}
}
