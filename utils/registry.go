package utils

import (
	"fmt"
	"gigo-ws/config"
	"strings"
)

// handleRegistryCaches
//
//	Checks if the container is from any of the source registries
//	and if so, replaces the host with container registry cache in the container
//	name. If there is a cache configured for docker.io and the container
//	contains no host then the container name is assumed to be from docker.io
func HandleRegistryCaches(containerName string, caches []config.RegistryCacheConfig) string {
	// create a variable to hold the docker.io cache if it exists
	var dockerCache config.RegistryCacheConfig

	// iterate over the registry caches
	for _, cache := range caches {
		// if the container name contains the registry host
		if strings.HasPrefix(containerName, cache.Source) {
			// replace the registry host with the cache host
			return strings.Replace(containerName, cache.Source, cache.Cache, 1)
		}

		// save the docker cache if it exists in case the container has no host prefix
		if cache.Source == "docker.io" {
			// set the docker cache
			dockerCache = cache
		}
	}

	// if the container name has no host prefix and the docker cache exists
	// then we assume the container is from docker.io and prepend the cache
	if dockerCache.Source == "docker.io" && strings.Count(containerName, "/") <= 1 {
		return fmt.Sprintf("%s/%s", dockerCache.Cache, containerName)
	}

	// return the container name if no cache was found
	return containerName
}
