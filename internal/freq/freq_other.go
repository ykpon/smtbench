//go:build !linux

package freq

func Detect() Source { return none{} }
