package storeowner

import "golang.org/x/sys/unix"

const ownershipTable = "/run/lock/goship2000-ownership-v1"
const ofdSetLock = unix.F_OFD_SETLK
