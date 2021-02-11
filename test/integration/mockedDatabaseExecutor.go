package integration

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"strconv"
	"testing"

	"github.com/Foundato/kelon/configs"
	"github.com/stretchr/testify/mock"
	"gopkg.in/yaml.v3"
)

type MockedDatastoreExecuter struct {
	mock.Mock
	counter   int
	responses DBTranslatorResponses
	t         *testing.T
}

func NewMockedDatastoreExecuter(t *testing.T) *MockedDatastoreExecuter {
	mocked := new(MockedDatastoreExecuter)
	mocked.On("Configure", mock.Anything, mock.Anything).Return(nil)
	mocked.On("Execute", mock.Anything, mock.Anything).Return(true, nil)

	mocked.counter = 0
	mocked.t = t

	response := &DBTranslatorResponses{}

	// Open config file
	inputBytes, err := ioutil.ReadFile("./test/integration/config/dbQueries.yml")
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

func (m *MockedDatastoreExecuter) Execute(statement interface{}, params []interface{}) (bool, error) {
	currentResponse := m.responses.Queries[strconv.Itoa(m.counter)]

	// statement map check for mongo datastores, sql datastores have simple string statement
	if reflect.ValueOf(statement).Kind() == reflect.Map {
		statementString := ""
		statementMap := statement.(map[string]string)
		for _, value := range []string{"apps", "users"} {
			if statementString == "" {
				statementString = fmt.Sprintf("%s, %s", value, statementMap[value])
			} else {
				statementString = fmt.Sprintf("%s, %s, %s", statementString, value, statementMap[value])
			}
		}

		// assert statement
		if statementString != currentResponse.Query {
			m.t.Errorf("Query %d does not match equal. Expected result %s /%s and translated result %s /%s ", m.counter, currentResponse.Query, currentResponse.Params, statementString, params)
			m.t.FailNow()
		}
	} else {
		// convert params slice to single string
		paramsString := ""
		for _, value := range params {
			if paramsString == "" {
				paramsString = value.(string)
			} else {
				paramsString = fmt.Sprintf("%s, %s", paramsString, value.(string))
			}
		}

		// assert statement and params
		if statement != currentResponse.Query && paramsString != currentResponse.Params {
			m.t.Errorf("Query %d does not match equal. Expected result %s /%s and translated result %s /%s ", m.counter, currentResponse.Query, currentResponse.Params, statement, paramsString)
			m.t.FailNow()
		}
	}

	m.counter++
	return true, nil
}

func (m *MockedDatastoreExecuter) Configure(appConf *configs.AppConfig, alias string) error {
	args := m.Called(appConf, alias)
	return args.Error(0)
}
