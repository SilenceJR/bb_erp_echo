package file

import "sync"

var moldAssetMutation sync.Mutex

// LockMoldAssetMutation serializes mold deletion/full import with mold image writes.
// It returns the unlock function so callers can defer it around owner validation and commit.
func LockMoldAssetMutation() func() {
	moldAssetMutation.Lock()
	return moldAssetMutation.Unlock
}
