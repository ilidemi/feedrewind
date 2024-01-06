package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

var Tailwind *cobra.Command

func init() {
	Tailwind = &cobra.Command{
		Use: "tailwind",
		Run: func(_ *cobra.Command, _ []string) {
			var tailwindPath string
			if runtime.GOARCH != "amd64" {
				panic(fmt.Errorf("Unknown arch: %s", runtime.GOARCH))
			}
			switch runtime.GOOS {
			case "windows":
				tailwindPath = "tailwind\\bin\\tailwindcss-3.0.23-windows-x64.exe"
			case "linux":
				tailwindPath = "tailwind/bin/tailwindcss-3.0.23-linux-x64"
			default:
				panic(fmt.Errorf("Unknown OS: %s", runtime.GOOS))
			}

			exeCmd := exec.Command(
				tailwindPath,
				"-i",
				"tailwind/application.tailwind.css",
				"-c",
				"tailwind/tailwind.config.js",
				"-o",
				"static/tailwind.css",
			)
			err := exeCmd.Run()
			if err != nil {
				panic(err)
			}
		},
	}
}
