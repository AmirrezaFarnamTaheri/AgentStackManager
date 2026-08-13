//go:build !windows

package main

func ensureElevated([]string, bool) (bool, error) { return false, nil }
