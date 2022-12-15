package integration

import (
	"context"
	"fmt"
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

type MockedDatastoreExecuter struct {
	mock.Mock
	counter   int
	responses DBTranslatorResponses
	t         *testing.T
	testName  string
}

func NewMockedDatastoreExecuter(t *testing.T, dbQueriesPath, testName string) *MockedDatastoreExecuter {
	mocked := new(MockedDatastoreExecuter)
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

func (m *MockedDatastoreExecuter) Execute(ctx context.Context, query data.DatastoreQuery) (bool, error) {
	currentResponse := m.responses.Queries[strconv.Itoa(m.counter)]

	// statement map check for mongo datastores, sql datastores have simple string statement
	if reflect.ValueOf(query.Statement).Kind() == reflect.Map {
		convertedStatement := query.Statement.(map[string]string)
		for key, value := range convertedStatement {
			expected, ok := currentResponse.Query[key]
			if !ok {
				m.t.Errorf("Testname: %s / Count %d : Did not expect a query with key [%s]", m.testName, m.counter, key)
				m.t.FailNow()
			}
			if !m.assertStrings(expected, value) {
				m.t.Errorf("Testname: %s / Count %d / Key %s : Query [%s] does not match expected result [%s]", m.testName, m.counter, key, value, expected)
				m.t.FailNow()
			}
		}
	} else if reflect.ValueOf(query.Statement).Kind() == reflect.String {
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
			m.t.Errorf("Testname: %s / Count %d : Did expect a query with key [sql]", m.testName, m.counter)
			m.t.FailNow()
		}
		if !m.assertStrings(expected, query.Statement.(string)) && !m.assertStrings(paramsString, currentResponse.Params) {
			m.t.Errorf("Testname: %s / Count %d : Query [%s / %s] does not match expected result [%s / %s]", m.testName, m.counter, query.Statement, paramsString, currentResponse.Query, currentResponse.Params)
			m.t.FailNow()
		}
	} else {
		m.t.Errorf("Testname: %s / Count %d : Unsupported Query type %T", m.testName, m.counter, query.Statement)
		m.t.FailNow()
	}
	m.counter++
	return true, nil
}

func (m *MockedDatastoreExecuter) assertStrings(expected, got string) bool {
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

func (m *MockedDatastoreExecuter) Configure(appConf *configs.AppConfig, alias string) error {
	args := m.Called(appConf, alias)
	return args.Error(0)
}
