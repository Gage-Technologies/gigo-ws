package provisioner

import (
	"context"
	"encoding/json"
	"fmt"
	"gigo-ws/utils"
	"path/filepath"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	tfjson "github.com/hashicorp/terraform-json"
)

// getTfVersion
//
//	Helper function to retrieve the terraform
//	version of the passed binary
func getTfVersion(binary string) (*version.Version, error) {
	// execute version command
	cmdOut, err := utils.ExecuteCommand(context.Background(), nil, "", binary, "version", "-json")
	if err != nil {
		return nil, fmt.Errorf("failed to execute tf version command: %v", err)
	}

	// ensure happy status code
	if cmdOut.ExitCode != 0 {
		return nil, fmt.Errorf(
			"tf version command returned non-zero exit code: %v\n    stdout: %s\n    stderr: %v",
			cmdOut.ExitCode, cmdOut.Stdout, cmdOut.Stderr,
		)
	}

	// marshall output from json
	var v tfjson.VersionOutput
	if err := json.Unmarshal([]byte(cmdOut.Stdout), &v); err != nil {
		return nil, fmt.Errorf(
			"tf version command returned invalid json: %v\n    stdout: %s\n    stderr: %v",
			err, cmdOut.Stdout, cmdOut.Stderr,
		)
	}

	// return version
	vrs, err := version.NewVersion(v.Version)
	if err != nil {
		return nil, fmt.Errorf(
			"tf version command returned invalid version: %v\n    stdout: %s\n    stderr: %v",
			err, cmdOut.Stdout, cmdOut.Stderr,
		)
	}

	return vrs, nil
}

// installTf
//
//	Installs the specified version of Terraform.
//
//	This function is idempotent, so if the version already exists,
//	it will not re-install it.
func installTf(ctx context.Context, vrs *version.Version, path string) error {
	// format version using the Hashicorp installer library
	installer := &releases.ExactVersion{
		Version:    vrs,
		InstallDir: path,
		Product:    product.Terraform,
	}

	// execute installer command
	bin, err := installer.Install(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute tf installer command: %v", err)
	}

	// ensure that the final install path and expected install path are correct
	if bin != filepath.Join(path, "terraform") {
		return fmt.Errorf("invalid path returned from tf install: %s != %s", bin, filepath.Join(path, "terraform"))
	}

	return nil
}
