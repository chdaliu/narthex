//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd

package termios

import (
	"bufio"
	"os"
)

// ReadPassword reads a line with echo on platforms without termios support.
func ReadPassword(prompt string) (string, error) {
	os.Stderr.WriteString(prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	line = line[:len(line)-1]
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line, nil
}
