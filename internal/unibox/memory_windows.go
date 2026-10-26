package unibox

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// memoryStatusEx is Windows' MEMORYSTATUSEX.
type memoryStatusEx struct {
	length, memoryLoad                               uint32
	totalPhys, availPhys, totalPageFile, availPage   uint64
	totalVirtual, availVirtual, availExtendedVirtual uint64
}

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

func hostMemory() (uint64, error) {
	s := memoryStatusEx{}
	s.length = uint32(unsafe.Sizeof(s))
	if ok, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&s))); ok == 0 {
		return 0, err
	}
	return s.totalPhys, nil
}
