//go:build !linux

package main

import "fmt"

func installLock() (func(), error)    { return nil, fmt.Errorf("installation requires Linux") }
func checkSpace(string, uint64) error { return fmt.Errorf("installation requires Linux") }
