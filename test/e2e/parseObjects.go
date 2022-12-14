package e2e

type Request struct {
	Name       string `yaml:"name"`
	URL        string `yaml:"url"`
	Body       string `yaml:"body"`
	StatusCode int    `yaml:"statusCode"`
}
