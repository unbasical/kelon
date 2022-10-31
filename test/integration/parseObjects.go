package integration

type DBTranslatorResponses struct {
	Queries map[string]DBQuery
}

type DBQuery struct {
	Query  map[string]string `yaml:"query"`
	Params string            `yaml:"params"`
	Text   string            `yaml:"text"`
}

type DBTranslatorRequests struct {
	Requests map[string]DBRequest
}

type DBRequest struct {
	Body           string `yaml:"body"`
	Text           string `yaml:"text"`
	ResponseStatus string `yaml:"responseStatus"`
	ThrowError     bool   `yaml:"error,omitempty"`
}

type DecisionBody struct {
	Allow bool `json:"result"`
}
