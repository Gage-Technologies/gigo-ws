package models

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gage-technologies/gigo-lib/storage"
	"github.com/gage-technologies/gigo-lib/utils"
)

// TerraformModule
//
//	Simple abstraction for terraform modules. A TerraformModule
//	is a simple main.tf HCL file and the module id that
//	corresponds to the module in the GIGO system database
type TerraformModule struct {
	MainTF      []byte
	ModuleID    int64
	Validated   bool
	LocalPath   string
	Environment []string
}

// LoadModule
//
//		Loads a gob encoded module from the storage engine
//	 If the module does not exist for the passed workspace id a nil module is returned
func LoadModule(storageEngine storage.Storage, workspaceId int64) (*TerraformModule, error) {
	// read module from the storage engine
	buf, _, err := storageEngine.GetFile(fmt.Sprintf("modules/%d", workspaceId))
	if err != nil {
		return nil, fmt.Errorf("failed to save module to storage engine: %v", err)
	}
	if buf == nil {
		return nil, nil
	}
	defer buf.Close()

	// decode buffer using gob
	var mod TerraformModule
	err = gob.NewDecoder(buf).Decode(&mod)
	if err != nil {
		return nil, fmt.Errorf("failed to decode module buffer: %v", err)
	}

	return &mod, nil
}

// DeleteModule
//
//	Helper function to delete a stored module
//	No-op if there is no module for the passed workspace id
func DeleteModule(storageEngine storage.Storage, workspaceId int64) error {
	// delete module from the storage engine
	err := storageEngine.DeleteFile(fmt.Sprintf("modules/%d", workspaceId))
	if err != nil {
		return fmt.Errorf("failed to delete module from storage engine: %v", err)
	}

	return nil
}

// StoreModule
//
//	Gob encodes the module and stores in the passed storage engine.
//	If there is an existing module written to the storage engine this
//	function will overwrite it.
func (m *TerraformModule) StoreModule(storageEngine storage.Storage) error {
	// remove transition and sensitive variables from module environment
	env := make([]string, 0)
	for _, e := range m.Environment {
		if strings.HasPrefix(e, "GIGO_WORKSPACE_TRANSITION") || strings.HasPrefix(e, "AWS_") {
			continue
		}
		env = append(env, e)
	}

	// copy module excluding the local directory
	c := &TerraformModule{
		MainTF:      m.MainTF,
		ModuleID:    m.ModuleID,
		Validated:   m.Validated,
		Environment: env,
	}

	// gob encode the module
	buf := new(bytes.Buffer)
	encoder := gob.NewEncoder(buf)
	err := encoder.Encode(c)
	if err != nil {
		return fmt.Errorf("failed to gob encode module: %v", err)
	}

	// save the module to the storage engine
	err = storageEngine.CreateFile(fmt.Sprintf("modules/%d", c.ModuleID), buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to save module to storage engine: %v", err)
	}

	return nil
}

// WriteTemporaryCopy
//
//	Writes a TerraformModule to a temporary directory on the local filesystem and
//	stores the local directory path inside the module. If a local directory already
//	exists and the main.tf matches the module, no action will be taken. If the
//	directory exists and the main.tf does not match the module, the function will
//	return an error.
func (m *TerraformModule) WriteTemporaryCopy() error {
	// handle module that has already been written
	if m.LocalPath != "" {
		// check if the main.tf matches the module
		fh, err := utils.HashFile(filepath.Join(m.LocalPath, "main.tf"))
		if err != nil {
			return fmt.Errorf("failed to hash main.tf file when checking existing module directory: %v", err)
		}

		mh, err := utils.HashData(m.MainTF)
		if err != nil {
			return fmt.Errorf("failed to hash module buffer when checking existing module directory: %v", err)
		}

		// raise hell for conflict
		if fh != mh {
			return fmt.Errorf("module %q already exists and the main.tf does not match the module", m.LocalPath)
		}

		return nil
	}

	// create a new temporary directory
	tempDir, err := os.MkdirTemp("/tmp", "gigo-ws-module-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %v", err)
	}

	// ensure permissions of the temporary directory
	err = os.Chmod(tempDir, 0700)
	if err != nil {
		return fmt.Errorf("failed to change permissions of temporary directory: %v", err)
	}

	// write the module to the temporary directory
	modulePath := filepath.Join(tempDir, "main.tf")
	err = os.WriteFile(modulePath, m.MainTF, 0600)
	if err != nil {
		return fmt.Errorf("failed to write module to temporary directory: %v", err)
	}

	// set local path
	m.LocalPath = tempDir

	return nil
}
