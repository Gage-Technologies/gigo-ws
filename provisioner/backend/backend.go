package backend

import "io"

type ProvisionerBackend interface {
	// String
	//
	//  Formats the provider backend into a string.
	//  Wrapper around ToTerraform to make native Go
	//  printing easier.
	String() string

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
	ToTerraform(bucketPath string) (string, []string)

	// GetStatefile
	//
	//  Returns the provisioner backend's current state file
	//  for the passed bucket path
	GetStatefile(bucketPath string) (io.ReadCloser, error)

	// RemoveStatefile
	//
	//  Removes the statefile and the backup statefile (if it exists) from
	//  the provisioner backend at the passed bucket path
	RemoveStatefile(bucketPath string) error
}
