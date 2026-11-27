package storeowner

// The shared lock inode must outlive every user-specific cache and TMPDIR.
const ownershipTable = "/Users/Shared/.goship2000-ownership-v1"

// Darwin sys/fcntl.h specifies F_OFD_SETLK = 90 (not POSIX F_SETLK,
// which releases this process's other claims when a descriptor closes).
const ofdSetLock = 90
