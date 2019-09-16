package configs

type ApiConfig struct {
	ResourcePools      []string                      `yaml:"resource_pools"`
	ResourcePoolGroups map[string]*ResourcePoolGroup `yaml:"resource_pool_groups"`
	ResourceLinks      map[string]string             `yaml:"resource_links"`
	JoinPaths          map[string][]string           `yaml:"join_paths"`
}

type ResourcePoolGroup struct {
	Resources []string
}
