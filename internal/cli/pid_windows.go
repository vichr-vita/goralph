//go:build windows

package cli

func pidAlive(pid int) bool {
	return pid > 0
}
