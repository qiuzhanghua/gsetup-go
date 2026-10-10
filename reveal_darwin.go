package main

import (
	"os/exec"
	"path/filepath"
)

// reveal 在 Finder 里把这个家亮出来。
func reveal(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return exec.Command("open", abs).Start()
}
