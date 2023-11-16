package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gigo-ws/config"
	"gigo-ws/models"
	"gigo-ws/provisioner/backend"
	utils2 "gigo-ws/utils"

	"github.com/gage-technologies/gigo-lib/logging"
	"github.com/gage-technologies/gigo-lib/utils"
	"github.com/hashicorp/go-version"
	tfjson "github.com/hashicorp/terraform-json"
)

// ApplyLogs
//
//	Buffered logs from a terraform apply
//	unmarshalled into maps
type ApplyLogs struct {
	StdOut []map[string]interface{}
	StdErr []map[string]interface{}
}

// DestroyLogs
//
//	Buffered logs from a terraform destroy
//	unmarshalled into maps
type DestroyLogs struct {
	StdOut []map[string]interface{}
	StdErr []map[string]interface{}
}

// Provisioner
//
//	Terraform provisioner used to manage
//	terraform assets of the GIGO system
type Provisioner struct {
	Backend          backend.ProvisionerBackend
	terraformPath    string
	terraformVersion *version.Version
	logger           logging.Logger
}

// NewProvisioner
//
//	Creates a new Provisioner and ensures that the
//	expected terraform binary is installed
func NewProvisioner(cfg config.ProvisionerConfig, logger logging.Logger) (*Provisioner, error) {
	// form binary path from terraform directory
	binaryPath := filepath.Join(cfg.TerraformDir, "terraform")

	// parse terraform version
	vrs, err := version.NewVersion(cfg.TerraformVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse terraform version: %v", err)
	}

	// check for existing terraform binary
	exists, err := utils.PathExists(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to check terraform path: %v", err)
	}

	// validate existing terraform install
	if exists {
		// ensure the passed binary is the correct version
		existingVersion, err := getTfVersion(binaryPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get terraform version: %v", err)
		}

		// handle version conflict
		if !vrs.Equal(existingVersion) {
			// exit with error if we don't have overwrite privileges
			if !cfg.Overwrite {
				return nil, fmt.Errorf("")
			}

			// remove existing install and overwrite `exists` to trigger new install
			err = os.Remove(binaryPath)
			if err != nil {
				return nil, fmt.Errorf("failed to remove existing terraform binary: %v", err)
			}
			exists = false
		}
	}

	// this has to stay as a separate if statement instead of an else for the
	// above conditional since we may we overwrite the `exists` variable on
	// a version conflict which would require this conditionals logic to execute
	if !exists {
		// ensure terraform directory exists
		err = os.MkdirAll(cfg.TerraformDir, 0700)
		if err != nil {
			return nil, fmt.Errorf("failed to create terraform directory: %v", err)
		}

		// perform terraform install via Hashicorp hc-install
		err = installTf(context.Background(), vrs, cfg.TerraformDir)
		if err != nil {
			return nil, fmt.Errorf("failed to install terraform: %v", err)
		}
	}

	// create provisioner Backend
	var provisionerBackend backend.ProvisionerBackend
	switch cfg.Backend.Type {
	case models.ProvisionerBackendFS:
		provisionerBackend, err = backend.NewProvisionerBackendFS(cfg.Backend.FS)
		if err != nil {
			return nil, fmt.Errorf("failed to create fs provisioner backend: %v", err)
		}
	case models.ProvisionerBackendS3:
		provisionerBackend, err = backend.NewProvisionerBackendS3(cfg.Backend.S3, cfg.Backend.InsecureS3)
		if err != nil {
			return nil, fmt.Errorf("failed to create s3 provisioner backend: %v", err)
		}
	default:
		return nil, fmt.Errorf("unknown provisioner Backend type: %d", provisionerBackend)
	}

	return &Provisioner{
		terraformPath:    binaryPath,
		terraformVersion: vrs,
		logger:           logger,
		Backend:          provisionerBackend,
	}, nil
}

// prepModule
//
//	Helper function to prep a module for terraform operations.
//
//	WARNING: This function will modify the <BACKEND_PROVIDER> template
//	the first time it is run on the module. THIS WILL MODIFY THE PASSED MODULE
func (p *Provisioner) prepModule(ctx context.Context, module *models.TerraformModule) error {
	p.logger.Debugf("prepping module: %d", module.ModuleID)

	// format module for write
	mod, envs := p.Backend.ToTerraform(fmt.Sprintf("states/%d", module.ModuleID))

	// override Backend template slot with Backend provider
	module.MainTF = bytes.ReplaceAll(
		module.MainTF,
		[]byte("<BACKEND_PROVIDER>"),
		[]byte(mod),
	)

	// update module environment variables
	module.Environment = append(module.Environment, envs...)

	// mark sure that module is written to local fs
	// this operation is idempotent so we execute every time
	err := module.WriteTemporaryCopy()
	if err != nil {
		return fmt.Errorf("failed to write module: %v", err)
	}

	// initialize terraform module
	// terraform implements init as an idempotent operation, so
	// we should execute this every time - the time cost is worth
	// the code cleanliness of not checking
	res, err := utils2.ExecuteCommand(
		ctx, module.Environment, "",
		"sh", "-c",
		fmt.Sprintf("%s -chdir=%s init", p.terraformPath, module.LocalPath),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize terraform module: %v", err)
	}

	if res.ExitCode != 0 {
		return fmt.Errorf("failed to initialize terraform module: %s", res.Stderr)
	}

	return nil
}

// Validate
//
//	Validates a terraform module and returns the terraform
//	validation output if there is an error
func (p *Provisioner) Validate(ctx context.Context, module *models.TerraformModule) (*tfjson.ValidateOutput, error) {
	p.logger.Debugf("validating module: %d", module.ModuleID)

	// prep module
	err := p.prepModule(ctx, module)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare module: %v", err)
	}

	// run terraform validate
	res, err := utils2.ExecuteCommand(
		ctx, module.Environment, "",
		"sh", "-c",
		fmt.Sprintf("%s -chdir=%s validate -json", p.terraformPath, module.LocalPath),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to validate module: %v", err)
	}

	// parse json from validation response
	var validationResponse tfjson.ValidateOutput
	err = json.Unmarshal([]byte(res.Stdout), &validationResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse validation response: %v", err)
	}

	// return error for invalid template
	if !validationResponse.Valid {
		return &validationResponse, fmt.Errorf("invalid terraform module")
	}

	if res.ExitCode != 0 {
		return nil, fmt.Errorf("failed to validate module: %s", res.Stderr)
	}

	// mark module as having been validated
	module.Validated = true

	return nil, nil
}

// Apply
//
//	Applies the passed terraform module
func (p *Provisioner) Apply(ctx context.Context, module *models.TerraformModule) (*ApplyLogs, error) {
	p.logger.Debugf("applying module: %d", module.ModuleID)

	// prep module
	err := p.prepModule(ctx, module)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare module: %v", err)
	}

	// run terraform apply
	res, err := utils2.ExecuteCommand(
		ctx, module.Environment, "",
		"sh", "-c",
		fmt.Sprintf(
			"TF_LOG=DEBUG %s -chdir=%s apply -json -auto-approve -no-color -input=false",
			p.terraformPath, module.LocalPath,
		),
	)

	// return error for invalid terraform module
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("failed to apply terraform module:\n    code: %d\n    out:\n%s\n    err:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// create apply result
	applyResult := &ApplyLogs{}

	// parse stdout jsonl from apply responses
	for _, l := range strings.Split(res.Stdout, "\n") {
		var m map[string]interface{}
		err = json.Unmarshal([]byte(l), &m)
		if err != nil {
			// make sure to wrap this error so that the caller
			// can check the type to determine if they should
			// proceed. this is important since apply executes
			// a resource state change and we don't want to
			// orphan resource cause we error here
			return nil, NewResultParseError(fmt.Sprintf("failed to parse apply response: %v", err))
		}
		applyResult.StdOut = append(applyResult.StdOut, m)
	}

	// b, _ := json.Marshal(applyResult.StdOut)
	// p.logger.Debugf("apply op internal logs %d:\n---\n%s\n---\n", module.ModuleID, string(b))

	// p.logger.Debugf("debug stderr %d\n---\n%s\n---", module.ModuleID, res.Stderr)

	// // parse stderr jsonl from apply responses
	// for _, l := range strings.Split(res.Stderr, "\n") {
	// 	var m map[string]interface{}
	// 	err = json.Unmarshal([]byte(l), &m)
	// 	if err != nil {
	// 		// make sure to wrap this error so that the caller
	// 		// can check the type to determine if they should
	// 		// proceed. this is important since apply executes
	// 		// a resource state change and we don't want to
	// 		// orphan resource cause we error here
	// 		return nil, NewResultParseError(fmt.Sprintf("failed to parse apply response: %v", err))
	// 	}
	// 	applyResult.StdErr = append(applyResult.StdErr, m)
	// }
	//
	// if len(applyResult.StdErr) > 0 {
	// 	b, _ = json.Marshal(applyResult.StdErr)
	// } else {
	// 	b = []byte("empty logs")
	// }
	// p.logger.Debugf("apply op internal err logs %d:\n---\n%s\n---\n", module.ModuleID, string(b))

	return applyResult, nil
}

// Destroy
//
//	Destroys the passed terraform module
func (p *Provisioner) Destroy(ctx context.Context, module *models.TerraformModule) (*DestroyLogs, error) {
	p.logger.Debugf("destroying module: %d", module.ModuleID)

	// prep module
	err := p.prepModule(ctx, module)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare module: %v", err)
	}

	// run terraform apply
	res, err := utils2.ExecuteCommand(
		ctx, module.Environment, "",
		"sh", "-c",
		fmt.Sprintf(
			"%s -chdir=%s destroy -json -auto-approve -no-color",
			p.terraformPath, module.LocalPath,
		),
	)

	// return error for invalid terraform module
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("failed to apply terraform module:\n    code: %d\n    out:\n%s\n    err:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// create destroy result
	destroyResult := &DestroyLogs{}

	// parse stdout jsonl from apply responses
	for _, l := range strings.Split(res.Stdout, "\n") {
		var m map[string]interface{}
		err = json.Unmarshal([]byte(l), &m)
		if err != nil {
			// make sure to wrap this error so that the caller
			// can check the type to determine if they should
			// proceed. this is important since destroy executes
			// a resource state change and we don't want to
			// orphan resource cause we error here
			return nil, NewResultParseError(fmt.Sprintf("failed to parse destroy response: %v", err))
		}
		destroyResult.StdOut = append(destroyResult.StdOut, m)
	}

	// // parse stderr jsonl from apply responses
	// for _, l := range strings.Split(res.Stderr, "\n") {
	// 	var m map[string]interface{}
	// 	err = json.Unmarshal([]byte(l), &m)
	// 	if err != nil {
	// 		// make sure to wrap this error so that the caller
	// 		// can check the type to determine if they should
	// 		// proceed. this is important since destroy executes
	// 		// a resource state change and we don't want to
	// 		// orphan resource cause we error here
	// 		return nil, NewResultParseError(fmt.Sprintf("failed to parse apply response: %v", err))
	// 	}
	// 	destroyResult.StdErr = append(destroyResult.StdErr, m)
	// }

	return destroyResult, nil
}
