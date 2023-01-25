package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/data"
)

// nolint: gochecknoglobals,gocritic
var dummyDatastoreConf = map[string]*configs.Datastore{
	"mongo": {
		Type:       data.TypeMongo,
		Connection: map[string]string{},
		Metadata:   map[string]string{},
	},
}

func Test_Operands_LoadDefault(t *testing.T) {
	_, err := LoadAllCallOperands(dummyDatastoreConf, nil)
	assert.NoError(t, err, "loading the default call operands should not result in an error")
}

func Test_Operands_LoadExternal(t *testing.T) {
	dirpath := "./testdata"
	handlers, err := LoadAllCallOperands(dummyDatastoreConf, &dirpath)
	assert.NoError(t, err, "loading the external call operands should not result in an error")

	eq, err := handlers["mongo"]["eq"]("one", "two")
	assert.NoError(t, err, "Mapping 'eq' should be included in call-ops mappings for mongo")
	assert.Equal(t, "override one two", eq)

	custom, err := handlers["mongo"]["custom"]("one")
	assert.NoError(t, err, "Mapping 'custom' should be included in call-ops mappings for mongo")
	assert.Equal(t, "custom one", custom)

	defaultEqual, err := handlers["mongo"]["equal"]("one", "two")
	assert.NoError(t, err, "Mapping 'equal' should be included in call-ops mapping for mongo")
	assert.Equal(t, "one: two", defaultEqual)
}

func Test_Operands_LoadNonExisting(t *testing.T) {
	dirpath := "./does-not-exist"
	_, err := LoadAllCallOperands(dummyDatastoreConf, &dirpath)
	assert.NoError(t, err, "no errors should be thrown. default call operands should be loaded")
}
