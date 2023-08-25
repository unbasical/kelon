package util

import "slices"

func SliceContainsSlice[T comparable](as, bs []T) (contains bool, notContained []T) {
	hm := make(map[T]struct{}, len(as))

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

func SliceMerge[T comparable](as, bs []T) []T {
	m := make(map[T]struct{})
	var merged = slices.Clone(as)

	for _, a := range as {
		m[a] = struct{}{}
	}

	for _, b := range bs {
		if _, ok := m[b]; !ok {
			merged = append(merged, b)
		}
	}

	return merged
}
