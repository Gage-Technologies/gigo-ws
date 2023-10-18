package api

import (
	"gigo-ws/config"
	"testing"
)

// TODO: figure out how to test this

func TestHandleRegistryCache(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		output        string
		config        []config.RegistryCacheConfig
	}{
		{
			name:          "no host hit",
			containerName: "test/test",
			output:        "localhost:5000/test/test",
			config: []config.RegistryCacheConfig{
				{
					Source: "docker.io",
					Cache:  "localhost:5000",
				},
			},
		},
		{
			name:          "host hit",
			containerName: "localhost:5000/test/test",
			output:        "localhost:8000/test/test",
			config: []config.RegistryCacheConfig{
				{
					Source: "localhost:5000",
					Cache:  "localhost:8000",
				},
			},
		},
		{
			name:          "no hit",
			containerName: "localhost:5000/test/test",
			output:        "localhost:5000/test/test",
			config: []config.RegistryCacheConfig{
				{
					Source: "localhost:5001",
					Cache:  "localhost:8000",
				},
			},
		},
		{
			name:          "no caches",
			containerName: "localhost:5000/test/test",
			output:        "localhost:5000/test/test",
			config:        []config.RegistryCacheConfig{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := handleRegistryCaches(test.containerName, test.config)
			if output != test.output {
				t.Errorf("expected %s, got %s", test.output, output)
			}
		})
	}
}
