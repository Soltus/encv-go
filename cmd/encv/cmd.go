//go:build !windows
// +build !windows

package main

import "github.com/spf13/cobra"

func addPlatformSpecificCommands(rootCmd *cobra.Command) {
	// 空实现
}
