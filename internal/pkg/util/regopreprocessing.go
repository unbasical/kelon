package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/pkg/constants/logging"
)

//nolint:gochecknoglobals,gocritic
var separator = "_"

func PreprocessPolicy(config *configs.AppConfig, rego string) string {
	for datastore, schemaMapping := range config.Data.DatastoreSchemas {
		for schema := range schemaMapping {
			rego = strings.ReplaceAll(rego, fmt.Sprintf("data.%s%s", schema, separator), fmt.Sprintf("data.%s.", datastore))
		}
	}

	return "# Preprocessed\n" + rego
}

func preprocessPolicyFile(config *configs.AppConfig, inPath, outPath string) {
	// read the whole file at once
	b, err := ioutil.ReadFile(inPath)
	if err != nil {
		logging.LogForComponent("regopreprocessing").Panic(err)
	}

	// check if output file exists
	var _, outErr = os.Stat(outPath)

	// create file if not exists
	if os.IsNotExist(outErr) {
		var file, createErr = os.Create(outPath)
		if createErr != nil {
			logging.LogForComponent("regopreprocessing").Panic(createErr)
		}
		defer file.Close()

		_, writeErr := file.WriteString(PreprocessPolicy(config, string(b)))
		if writeErr != nil {
			logging.LogForComponent("regopreprocessing").Panic(writeErr)
		}
		return
	}

	// write the whole body at once
	err = ioutil.WriteFile(outPath, []byte(PreprocessPolicy(config, string(b))), 0644)
	if err != nil {
		logging.LogForComponent("regopreprocessing").Panic(err)
	}
}

func PrepocessPoliciesInDir(config *configs.AppConfig, dir string) string {
	outDir := "/tmp/policies"
	err := os.MkdirAll(outDir, 0777)
	if err != nil {
		logging.LogForComponent("regopreprocessing").Panic(err)
	}

	// Load regos
	var files []string
	err = filepath.Walk(dir, func(path string, f os.FileInfo, _ error) error {
		if !f.IsDir() {
			if filepath.Ext(f.Name()) == ".rego" {
				files = append(files, dir+"/"+f.Name())
			}
		}
		return nil
	})
	if err != nil {
		logging.LogForComponent("regopreprocessing").Errorf("Error while preprocessing policies: %s", err.Error())
	}

	// Process & write back
	for _, regoPath := range files {
		preprocessPolicyFile(config, regoPath, strings.ReplaceAll(regoPath, dir, outDir))
	}

	return outDir
}
