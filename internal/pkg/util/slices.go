package util

func SliceContains[T comparable](s []T, v T) bool {
	for _, vs := range s {
		if v == vs {
			return true
		}
	}
	return false
}

func SliceContainsSlice[T comparable](as []T, bs []T) (bool, []T) {
	hm := make(map[T]struct{}, len(as))
	var notContained []T

	for _, a := range as {
		hm[a] = struct{}{}
	}

	for _, b := range bs {
		if _, ok := hm[b]; !ok {
			notContained = append(notContained, b)
		}
	}

	return len(notContained) == 0, notContained
}

func SliceMerge[T comparable](as []T, bs []T) []T {
	m := make(map[T]struct{})
	var merged []T

	for _, a := range as {
		m[a] = struct{}{}
		merged = append(merged, a)
	}

	for _, b := range bs {
		if _, ok := m[b]; !ok {
			merged = append(merged, b)
		}
	}

	return merged
}
