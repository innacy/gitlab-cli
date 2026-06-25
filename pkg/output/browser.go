package output

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

var browserEnabled bool

// SetBrowserOpen configures whether PrintURLOpen should auto-open URLs.
func SetBrowserOpen(enabled bool) {
	browserEnabled = enabled
}

// PrintURLOpen prints a URL to the terminal and opens it in the default browser
// when enabled. It only opens when stdout is a terminal (TTY).
func PrintURLOpen(url string) {
	PrintURL(url)
	if browserEnabled && isTTY() {
		openBrowser(url)
	}
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "  (could not open browser: %v)\n", err)
		return
	}
	go cmd.Wait()
}
