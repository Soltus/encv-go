//go:build windows
// +build windows

package main

import (
	"github.com/spf13/cobra"
)

// addPlatformSpecificCommands 在 Windows 平台下添加特定命令
func addPlatformSpecificCommands(rootCmd *cobra.Command) {
	rootCmd.AddCommand(registerProtocolCmd)
	rootCmd.AddCommand(unregisterProtocolCmd)
	rootCmd.AddCommand(registerCmd)
	rootCmd.AddCommand(unregisterCmd)
	rootCmd.AddCommand(openasCmd)
	rootCmd.AddCommand(openStreamCmd)
}
