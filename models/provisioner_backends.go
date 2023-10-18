package models

type ProvisionerBackendType int

const (
	// ProvisionerBackendFS is the provisioner backend type for locally accessible file systems
	ProvisionerBackendFS ProvisionerBackendType = iota
	// ProvisionerBackendS3 is the provisioner backend type for S3 compliant storage
	ProvisionerBackendS3
)

func (t *ProvisionerBackendType) String() string {
	switch *t {
	case ProvisionerBackendFS:
		return "fs"
	case ProvisionerBackendS3:
		return "s3"
	}
	return "unknown"
}
