package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"rye/internal/atomicfile"
)

// SSHInclude is the line in ~/.ssh/config that reads rye's SSH aliases. ssh uses the first value it finds for each
// option, and the line goes at the top, ahead of any Host block.
const SSHInclude = "Include config.d/rye"

// WriteSSHConfig writes config, rye's SSH aliases, to ~/.ssh/config.d/rye under home, and reports whether
// ~/.ssh/config has SSHInclude. It leaves ~/.ssh/config to the person.
func WriteSSHConfig(home, config string) (included bool, err error) {
	dir := filepath.Join(home, ".ssh", "config.d")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	if err := atomicfile.Write(filepath.Join(dir, "rye"), []byte(config)); err != nil {
		return false, err
	}
	current, err := os.ReadFile(filepath.Clean(filepath.Join(home, ".ssh", "config")))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(current), "\n") {
		if strings.TrimSpace(line) == SSHInclude {
			return true, nil
		}
	}
	return false, nil
}
