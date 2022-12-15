package request

import (
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/request"
)

type pathMapper struct {
	appConf    *configs.AppConfig
	mappings   []*compiledMapping
	configured bool
}

type compiledMapping struct {
	matcher        *regexp.Regexp
	mapping        *configs.APIMapping
	authorization  bool
	authentication bool
	importance     int
	datastores     []string
}

type pathMapperInput struct {
	Method string
	URL    *url.URL
}

// New instance of a request.PathMapper that handles REST-like paths.
func NewPathMapper() request.PathMapper {
	return &pathMapper{
		appConf:    nil,
		configured: false,
	}
}

// See request.PathMapper.
func (mapper *pathMapper) Configure(appConf *configs.AppConfig) error {
	// Exit if already configured
	if mapper.configured {
		return nil
	}

	if appConf == nil {
		return errors.Errorf("PathMapper: AppConfig not configured!")
	}
	mapper.appConf = appConf
	if err := mapper.generateMappings(); err != nil {
		return errors.Wrap(err, "PathMapper: Error while parsing config")
	}
	mapper.configured = true
	logging.LogForComponent("pathMapper").Infoln("Configured PathMapper")
	return nil
}

// See request.PathMapper
func (mapper pathMapper) Map(input interface{}) (*request.MapperOutput, error) {
	if !mapper.configured {
		return nil, errors.Errorf("PathMapper was not configured! Please call Configure(). ")
	}

	// Check type and handle request
	switch in := input.(type) {
	case *pathMapperInput:
		if in.URL == nil {
			return nil, errors.Errorf("PathMapper: Argument URL mustn't be nil! ")
		}
		if in.Method == "" {
			return nil, errors.Errorf("PathMapper: Argument Method mustn't be empty! ")
		}
		return mapper.handleInput(in)
	default:
		return nil, errors.Errorf("PathMapper: Input of Process() was not of type *request.pathMapperInput! Type was: " + reflect.TypeOf(input).String())
	}
}

func (mapper pathMapper) handleInput(input *pathMapperInput) (*request.MapperOutput, error) {
	var matches []*compiledMapping
	requestString := fmt.Sprintf("%s-%s", input.Method, input.URL.Path)
	for _, mapping := range mapper.mappings {
		if mapping.matcher.MatchString(requestString) {
			matches = append(matches, mapping)
		}
	}

	// Sort by importance descending
	if len(matches) > 0 {
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].importance > matches[j].importance
		})

		// Throw error if ambiguous paths are matched
		if len(matches) > 1 && matches[0].importance == matches[1].importance {
			return nil, request.PathAmbiguousError{
				RequestURL: requestString,
				FirstMatch: matches[0].mapping.Path,
				OtherMatch: matches[1].mapping.Path,
			}
		}
		logging.LogForComponent("pathMapper").Debugf("Found matching API-Mapping [%s]", matches[0].matcher.String())

		// Match found
		return &request.MapperOutput{
			Datastores:     matches[0].datastores,
			Package:        matches[0].mapping.Package,
			Authentication: matches[0].authentication,
			Authorization:  matches[0].authorization,
		}, nil
	}

	// No matches at all
	return nil, request.PathNotFoundError{
		RequestURL: requestString,
	}
}

func (mapper *pathMapper) generateMappings() error {
	for _, dsMapping := range mapper.appConf.API.Mappings {
		pathPrefix := dsMapping.Prefix
		for _, mapping := range dsMapping.Mappings {
			endpointsRegex := "[(GET)|(POST)|(PUT)|(DELETE)|(PATCH)]"
			endpointsCount := 0
			if mapping.Methods != nil && len(mapping.Methods) > 0 {
				endpointsCount = len(mapping.Methods)
				anchoredMappings := make([]string, endpointsCount)
				for i, method := range mapping.Methods {
					anchoredMappings[i] = fmt.Sprintf("(%s)", method)
				}
				endpointsRegex = strings.ToUpper(fmt.Sprintf("[%s]", strings.Join(anchoredMappings, "|")))
			}

			queriesRegex := ""
			queriesCount := 0
			if mapping.Queries != nil && len(mapping.Queries) > 0 {
				queriesRegex = fmt.Sprintf("?%s=.*?", strings.Join(mapping.Queries, "=.*?"))
				queriesCount = len(mapping.Queries)
			}

			regex, err := regexp.Compile(fmt.Sprintf("%s-%s%s%s", endpointsRegex, pathPrefix, mapping.Path, queriesRegex))
			if err != nil {
				return errors.Wrap(err, "PathMapper: Error during parsing config")
			}

			mapper.mappings = append(mapper.mappings, &compiledMapping{
				matcher:        regex,
				mapping:        mapping,
				authentication: *dsMapping.Authentication,
				authorization:  *dsMapping.Authorization,
				importance:     len(pathPrefix) + len(mapping.Path) + queriesCount + endpointsCount,
				datastores:     dsMapping.Datastores,
			})
		}
	}
	return nil
}
