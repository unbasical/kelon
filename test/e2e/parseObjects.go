package e2e

// Request represents a single E2E test
type Request struct {
	Name       string            `yaml:"name"`
	Method     string            `yaml:"method"`
	URL        string            `yaml:"url"`
	Body       string            `yaml:"body"`
	StatusCode int               `yaml:"statusCode"`
	Headers    map[string]string `yaml:"header"`
}

// Defaults sets default values for required properties, if they are not set
func (r *Request) Defaults() {
	if r.Method == "" {
		r.Method = "POST"
	}
}
