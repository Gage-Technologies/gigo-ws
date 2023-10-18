package provisioner

import (
	"context"
	"github.com/hashicorp/go-version"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGetTfVersion(t *testing.T) {
	// generate path to base repo directory
	_, b, _, _ := runtime.Caller(0)
	basepath := strings.Replace(filepath.Dir(b), "/provisioner", "", -1)

	vrs, err := getTfVersion(filepath.Join(basepath, "test_data/tf/tf-1.3.7"))
	if err != nil {
		t.Fatal(err)
	}

	if vrs.String() != "1.3.7" {
		t.Errorf("expected %q, got %q", "1.3.7", vrs.String())
	}
}

func TestInstallTf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		vrs  string
	}{
		{
			name: "1",
			path: "/tmp/gigo-ws-tf-install-test-1",
			vrs:  "1.3.7",
		},
		{
			name: "2",
			path: "/tmp/gigo-ws-tf-install-test-2",
			vrs:  "1.3.5",
		},
		{
			name: "3",
			path: "/tmp/gigo-ws-tf-install-test-3",
			vrs:  "1.3.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			vrs, err := version.NewVersion(tt.vrs)
			if err != nil {
				t.Fatal(err)
			}

			err = os.MkdirAll(tt.path, 0744)
			if err != nil {
				t.Fatal(err)
			}

			defer os.RemoveAll(tt.path)

			err = installTf(ctx, vrs, tt.path)
			if err != nil {
				t.Error(err)
			}

			iVrs, err := getTfVersion(tt.path + "/terraform")
			if err != nil {
				t.Fatal(err)
			}

			if !vrs.Equal(iVrs) {
				t.Fatalf("incorrect version installed %s != %s", iVrs, vrs)
			}
		})
	}
}
