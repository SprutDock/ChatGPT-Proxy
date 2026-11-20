package platform

import "errors"

var ErrProxyOwnerConflict = errors.New("local proxy port is owned by another process")
