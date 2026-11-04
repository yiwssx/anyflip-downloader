//go:build !linux

package main

// Non-Linux builds safely omit filesystem statistics; Admin API remains
// available without making claims about host storage capacity.
func readAdminStorage(root string) *adminStorageRecord { return nil }
