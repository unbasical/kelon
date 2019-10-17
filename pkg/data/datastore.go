package data

import (
	"github.com/Foundato/kelon/configs"
)

type Datastore interface {
	Configure(appConf *configs.AppConfig, alias string) error
	Execute(query *Node) (bool, error)
}

type CallOpMapper interface {
	Handles() string
	Map(args ...string) string
}
