package platform

import "errors"

var (
	ErrProxyOwnerConflict     = errors.New("local proxy port is owned by another process")
	ErrStunnelPathUnavailable = errors.New("could not safely prepare the stunnel state path")
)
