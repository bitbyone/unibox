//go:build !darwin && !linux && !windows

package unibox

import "errors"

func hostMemory() (uint64, error) {
	return 0, errors.New("unibox reads the memory of macOS, Linux and Windows alone")
}
