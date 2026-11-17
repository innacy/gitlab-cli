package config

import "os"

// ponytail: no cheap owner check on Windows; the regular-file check still applies.
func ownedByCurrentUser(os.FileInfo) bool { return true }
