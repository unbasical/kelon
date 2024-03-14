package e2e

type Request struct {
	Name       string            `yaml:"name"`
	Method     string            `yaml:"method"`
	URL        string            `yaml:"url"`
	Body       string            `yaml:"body"`
	StatusCode int               `yaml:"statusCode"`
	Headers    map[string]string `yaml:"header"`
}

func (r *Request) Defaults() {
	if r.Method == "" {
		r.Method = "POST"
	}
}
