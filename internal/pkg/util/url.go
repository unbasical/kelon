package util

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

func RelativeFileURLToAbsolute(u url.URL) url.URL {
	if u.Scheme == "file" {
		s := strings.TrimPrefix(u.String(), "file://")
		a, err := filepath.Abs(s)
		if err != nil {
			panic(err)
		}

		return *MustParseURL(fmt.Sprintf("file://%s", a))
	}
	return u
}

func MustParseURL(u string) *url.URL {
	uu, err := url.Parse(u)
	if err != nil {
		panic(err)
	}
	return uu
}
