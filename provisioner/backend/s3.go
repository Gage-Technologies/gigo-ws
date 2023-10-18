package backend

import (
	"fmt"
	"github.com/gage-technologies/gigo-lib/config"
	"github.com/gage-technologies/gigo-lib/storage"
	"io"
)

const provisionerBackendS3Template = `backend "s3" {
  bucket = "%s"
  region = "%s"
  endpoint = "%s"
  key = "%s"
}`

const provisionerBackendS3InsecureTemplate = `backend "s3" {
  bucket = "%s"
  region = "%s"
  endpoint = "%s"
  key = "%s"
  skip_credentials_validation = true
  skip_metadata_api_check = true
  skip_region_validation = true
  force_path_style = true
}`

// ProvisionerBackendS3
//
//	S3 based implementation of the Terraform
//	remote backend
type ProvisionerBackendS3 struct {
	config.StorageS3Config
	insecure      bool
	storageEngine *storage.MinioObjectStorage
}

// NewProvisionerBackendS3
//
//	Creates a new ProvisionerBackendS3 from as S3 storage configuration
func NewProvisionerBackendS3(c config.StorageS3Config, insecureS3 bool) (ProvisionerBackend, error) {
	// create storage engine that correlated to provisioner backend
	storageEngine, err := storage.CreateMinioObjectStorage(c)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage engine: %v", err)
	}
	return &ProvisionerBackendS3{
		StorageS3Config: c,
		insecure:        insecureS3,
		storageEngine:   storageEngine,
	}, nil
}

// String
//
//	Formats the provider backend into a string.
//	Wrapper around ToTerraform to make native Go
//	printing easier.
func (b *ProvisionerBackendS3) String() string {
	// create a new provisioner backend without sensitive fields
	sanitized := &ProvisionerBackendS3{
		StorageS3Config: b.StorageS3Config,
	}
	sanitized.SecretKey = ""
	sanitized.AccessKey = ""

	// format to the terraform HCL configuration string but pass an empty value
	// for the bucket path since we have no specific target
	s, _ := sanitized.ToTerraform("")
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
func (b *ProvisionerBackendS3) ToTerraform(bucketPath string) (string, []string) {
	template := provisionerBackendS3Template
	if b.insecure {
		template = provisionerBackendS3InsecureTemplate
	}

	endpoint := b.Endpoint
	if !b.UseSSL {
		endpoint = "http://" + endpoint
	}
	return fmt.Sprintf(
			template,
			b.Bucket,
			b.Region,
			endpoint,
			bucketPath,
		), []string{
			fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", b.AccessKey),
			fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", b.SecretKey),
		}
}

// GetStatefile
//
//	Returns the provisioner backend's current state file
//	for the passed bucket path
func (b *ProvisionerBackendS3) GetStatefile(bucketPath string) (io.ReadCloser, error) {
	return b.storageEngine.GetFile(bucketPath)
}

// RemoveStatefile
//
//	Removes the statefile and the backup statefile (if it exists) from
//	the provisioner backend at the passed bucket path
func (b *ProvisionerBackendS3) RemoveStatefile(bucketPath string) error {
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
