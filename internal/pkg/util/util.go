package util

func AllContained(base []string, search []string) bool {
	contained := true
	for _, el := range search {
		if !Contains(base, el) {
			contained = false
		}
	}

	return contained
}

func Contains(base []string, search string) bool {
	for _, x := range base {
		if search == x {
			return true
		}
	}

	return false
}
