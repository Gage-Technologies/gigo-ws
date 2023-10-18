package backend

import (
	"fmt"
	"github.com/gage-technologies/gigo-lib/config"
	"github.com/gage-technologies/gigo-lib/storage"
	"io"
	"path/filepath"
)

const provisionerBackendFSTemplate = `backend "local" {
  path = "%s"
}`

// ProvisionerBackendFS
//
//	Filesystem based implementation of the Terraform
//	remote backend
type ProvisionerBackendFS struct {
	config.StorageFSConfig
	storageEngine storage.Storage
}

// NewProvisionerBackendFS
//
//	Creates a new ProvisionerBackendFS from as FS storage configuration
func NewProvisionerBackendFS(c config.StorageFSConfig) (ProvisionerBackend, error) {
	// create storage engine that correlates to the provisioner backend
	storageEngine, err := storage.CreateFileSystemStorage(c.Root)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage engine: %v", err)
	}

	return &ProvisionerBackendFS{
		StorageFSConfig: c,
		storageEngine:   storageEngine,
	}, nil
}

// String
//
//	Formats the provider backend into a string.
//	Wrapper around ToTerraform to make native Go
//	printing easier.
func (b *ProvisionerBackendFS) String() string {
	// format to the terraform HCL configuration string but pass an empty value
	// for the bucket path since we have no specific target
	s, _ := b.ToTerraform("")
	return s
}

// ToTerraform
//
//		Formats the provider backend into a Terraform
//		compatible backend configuration that can be
//		inserted into a terraform HCL file
//
//	 Args
//			- bucketPath (string): path to terraform state inside the bucket
//	 Returns
//	     (string): terraform HCL compliant backend configuration
//		 ([]string): credentials in the form of environment variables
func (b *ProvisionerBackendFS) ToTerraform(bucketPath string) (string, []string) {
	return fmt.Sprintf(
		provisionerBackendFSTemplate,
		filepath.Join(b.Root, "states", bucketPath),
	), []string{}
}

// GetStatefile
//
//	Returns the provisioner backend's current state file
//	for the passed bucket path
func (b *ProvisionerBackendFS) GetStatefile(bucketPath string) (io.ReadCloser, error) {
	return b.storageEngine.GetFile(bucketPath)
}

// RemoveStatefile
//
//	Removes the statefile and the backup statefile (if it exists) from
//	the provisioner backend at the passed bucket path
func (b *ProvisionerBackendFS) RemoveStatefile(bucketPath string) error {
	// delete statefile
	err := b.storageEngine.DeleteFile(bucketPath)
	if err != nil {
		return fmt.Errorf("failed to delete state file: %v", err)
	}

	// check for backup
	exists, _, err := b.storageEngine.Exists(bucketPath + ".backup")
	if err != nil {
		return fmt.Errorf("failed to check for backup file: %v", err)
	}

	// remove backup state if it exists
	if exists {
		err = b.storageEngine.DeleteFile(bucketPath + ".backup")
		if err != nil {
			return fmt.Errorf("failed to delete backup file: %v", err)
		}
	}

	return nil
}
