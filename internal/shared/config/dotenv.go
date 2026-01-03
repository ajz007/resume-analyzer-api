package config

import (
	"bufio"
	"os"
	"strings"
)

// loadEnvFiles loads simple KEY=VALUE pairs from the given files if they exist.
// It is a best-effort helper for local development; errors are ignored.
func loadEnvFiles(paths ...string) {
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `"`)
			if key != "" {
				os.Setenv(key, val)
			}
		}
		_ = f.Close()
	}
}
