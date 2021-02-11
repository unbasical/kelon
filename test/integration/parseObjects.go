package integration

type DBTranslatorResponses struct {
	Queries map[string]DBQuery
}

type DBQuery struct {
	Query  string `yaml:"query"`
	Params string `yaml:"params"`
	Text   string `yaml:"text"`
}

type DBTranslatorRequests struct {
	Requests map[string]DBRequest
}

type DBRequest struct {
	Body           string `yaml:"body"`
	Text           string `yaml:"text"`
	ResponseStatus string `yaml:"responseStatus"`
}
