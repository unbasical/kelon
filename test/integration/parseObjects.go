package integration

// DBTranslatorResponses hold all database queries used in the tests
// The query ID will be used in error cases for better identify single failing tests
type DBTranslatorResponses struct {
	Queries map[string]DBQuery
}

// DBQuery represents a single db query to be executed. The Query, its Params and the name of the test (Text) it should be associated with
type DBQuery struct {
	Query  map[string]string `yaml:"query"`
	Params string            `yaml:"params"`
	Text   string            `yaml:"text"`
}

// DBTranslatorRequests hold all integration test by ID, which will be used in error cases for better identify single failing tests
type DBTranslatorRequests struct {
	Requests map[string]DBRequest
}

// DBRequest represents a single integration test consisting of the input Body, the test name (Text) and whether the test
// is expected to succeed or fail (Success)
type DBRequest struct {
	Body    string `yaml:"body"`
	Text    string `yaml:"text"`
	Success bool   `yaml:"success"`
}
