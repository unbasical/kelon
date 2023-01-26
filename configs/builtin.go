package configs

type Builtin struct {
	Extension string                 `yaml:"extension"`
	Config    map[string]interface{} `yaml:"config"`
}
