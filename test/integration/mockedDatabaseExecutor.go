package integration

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/data"
	"gopkg.in/yaml.v3"
)

// MockedDatastoreExecutor is a data.DatastoreExecutor implementation, which gets database queries to expect and asserts
// incoming queries match the pre-defined ones
type MockedDatastoreExecutor struct {
	mock.Mock
	counter   int
	responses DBTranslatorResponses
	t         *testing.T
	testName  string
}

// NewMockedDatastoreExecuter creates a new mocking executer
func NewMockedDatastoreExecuter(t *testing.T, dbQueriesPath, testName string) *MockedDatastoreExecutor {
	mocked := new(MockedDatastoreExecutor)
	mocked.testName = testName
	mocked.On("Configure", mock.Anything, mock.Anything).Return(nil)
	mocked.On("Execute", mock.Anything, mock.Anything).Return(true, nil)

	mocked.counter = 0
	mocked.t = t

	response := &DBTranslatorResponses{}

	// Open config file
	inputBytes, err := os.ReadFile(dbQueriesPath)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	// Parse config from yaml to object
	err = yaml.Unmarshal(inputBytes, response)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	mocked.responses = *response
	return mocked
}

// Execute - see data.DatastoreExecutor
func (m *MockedDatastoreExecutor) Execute(_ context.Context, query data.DatastoreQuery) (bool, error) {
	currentResponse := m.responses.Queries[strconv.Itoa(m.counter)]

	// statement map check for mongo datastores, sql datastores have simple string statement
	var err error
	switch reflect.ValueOf(query.Statement).Kind() {
	case reflect.Map:
		err = m.assertMongo(currentResponse, query)
	case reflect.String:
		err = m.assertSql(currentResponse, query)
	default:
		err = errors.Errorf("Testname: %s / Count %d : Unsupported Query type %T", m.testName, m.counter, query.Statement)
	}

	// Check assertion didn't fail
	if err != nil {
		m.t.Error(err)
		m.t.FailNow()
	}

	m.counter++
	return true, nil
}

// assertMongo validates queries for MongoDB
func (m *MockedDatastoreExecutor) assertMongo(currentResponse DBQuery, query data.DatastoreQuery) error {
	convertedStatement := query.Statement.(map[string]string)
	for key, value := range convertedStatement {
		// assert statement and params
		expected, ok := currentResponse.Query[key]
		if !ok {
			return errors.Errorf("Testname: %s / Count %d : Did not expect a query with key [%s]", m.testName, m.counter, key)
		}
		if !m.assertStrings(expected, value) {
			return errors.Errorf("Testname: %s / Count %d / Key %s : Query [%s] does not match expected result [%s]", m.testName, m.counter, key, value, expected)
		}
	}
	return nil
}

// assertSql validates queries for SQL based databases
func (m *MockedDatastoreExecutor) assertSql(currentResponse DBQuery, query data.DatastoreQuery) error {
	// convert params slice to single string
	paramsString := ""
	for _, value := range query.Parameters {
		if paramsString == "" {
			paramsString = value.(string)
		} else {
			paramsString = fmt.Sprintf("%s, %s", paramsString, value.(string))
		}
	}

	// assert statement and params
	expected, ok := currentResponse.Query["sql"]
	if !ok {
		return errors.Errorf("Testname: %s / Count %d : Did expect a query with key [sql]", m.testName, m.counter)
	}
	if !m.assertStrings(expected, query.Statement.(string)) && !m.assertStrings(paramsString, currentResponse.Params) {
		return errors.Errorf("Testname: %s / Count %d : Query [%s / %s] does not match expected result [%s / %s]", m.testName, m.counter, query.Statement, paramsString, currentResponse.Query, currentResponse.Params)
	}
	return nil
}

func (m *MockedDatastoreExecutor) assertStrings(expected, got string) bool {
	for _, specialRune := range strings.Split(",:;\"'()[]{}", "") {
		expected = strings.ReplaceAll(expected, specialRune, "")
		got = strings.ReplaceAll(got, specialRune, "")
	}

	expectedTokens := strings.Split(expected, " ")
	sort.Strings(expectedTokens)

	gotTokens := strings.Split(got, " ")
	sort.Strings(gotTokens)

	if len(expectedTokens) != len(gotTokens) {
		return false
	}
	for i, v := range expectedTokens {
		if v != gotTokens[i] {
			return false
		}
	}
	return true
}

// Configure - see data.DatastoreExecutor
func (m *MockedDatastoreExecutor) Configure(appConf *configs.AppConfig, alias string) error {
	args := m.Called(appConf, alias)
	return args.Error(0)
}
