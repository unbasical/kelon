package request

import (
	"fmt"
	"github.com/Foundato/kelon/configs"
)

type PathMapper interface {
	Configure(appConf *configs.AppConfig) error
	Map(interface{}) (*MapperOutput, error)
}

type PathAmbiguousError struct {
	RequestUrl string
	FirstMatch string
	OtherMatch string
}

type PathNotFoundError struct {
	RequestUrl string
}

type MapperOutput struct {
	Datastore string
	Package   string
}

func (e *PathAmbiguousError) Error() string {
	return fmt.Sprintf("Path-mapping [%s] is ambiguous! Mapping [%s] also matches incoming path [%s]!", e.RequestUrl, e.FirstMatch, e.OtherMatch)
}

func (e *PathNotFoundError) Error() string {
	return fmt.Sprintf("PathMapper: There is no mapping which matches path [%s]!", e.RequestUrl)
}
