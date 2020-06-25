package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Foundato/kelon/configs"

	log "github.com/sirupsen/logrus"
)

//nolint:gochecknoglobals
var separator = "_"

func PreprocessPolicy(config *configs.AppConfig, rego string) string {
	for datastore, schemaMapping := range config.Data.DatastoreSchemas {
		for schema := range schemaMapping {
			rego = strings.ReplaceAll(rego, fmt.Sprintf("data.%s%s", schema, separator), fmt.Sprintf("data.%s.", datastore))
		}
	}

	return "# Preprocessed\n" + rego
}

func preprocessPolicyFile(config *configs.AppConfig, inPath string, outPath string) {
	// read the whole file at once
	b, err := ioutil.ReadFile(inPath)
	if err != nil {
		log.Panic(err)
	}

	// check if output file exists
	var _, outErr = os.Stat(outPath)

	// create file if not exists
	if os.IsNotExist(outErr) {
		var file, createErr = os.Create(outPath)
		if createErr != nil {
			log.Panic(createErr)
		}
		defer file.Close()

		_, writeErr := file.WriteString(PreprocessPolicy(config, string(b)))
		if writeErr != nil {
			log.Panic(writeErr)
		}
		return
	}

	// write the whole body at once
	err = ioutil.WriteFile(outPath, []byte(PreprocessPolicy(config, string(b))), 0644)
	if err != nil {
		log.Panic(err)
	}
}

func PrepocessPoliciesInDir(config *configs.AppConfig, dir string) string {
	outDir := "/tmp/policies"
	err := os.MkdirAll(outDir, 0777)
	if err != nil {
		log.Panic(err)
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
		log.Errorf("Error while preprocessing policies: %s", err.Error())
	}

	// Process & write back
	for _, regoPath := range files {
		preprocessPolicyFile(config, regoPath, strings.ReplaceAll(regoPath, dir, outDir))
	}

	return outDir
}
