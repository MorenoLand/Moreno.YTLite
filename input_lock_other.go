//go:build !windows

package main

func setInputLocked(bool)         {}
func installInputLockHook() error { return nil }
func uninstallInputLockHook()     {}
