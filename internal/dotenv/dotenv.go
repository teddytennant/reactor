// Package dotenv loads .env into the process environment. Reactor keeps
// provider credentials (Fireworks, xAI, Daytona) in .env rather than in code or
// config so that a key is never committed and never reaches a chamber process —
// internal/procenv strips them again before anything untrusted is spawned.
package dotenv

import (
	"bufio"
	"os"
	"strings"
)

// Load reads a .env file and sets any variable not already in the environment.
// Real environment variables always win, so `FOO=x reactor` overrides the file.
// A missing file is not an error.
func Load(paths ...string) {
	if len(paths) == 0 {
		paths = []string{".env"}
	}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			k, v, ok := parseLine(sc.Text())
			if !ok {
				continue
			}
			if _, exists := os.LookupEnv(k); !exists {
				os.Setenv(k, v)
			}
		}
		f.Close()
	}
}

func parseLine(line string) (key, val string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	i := strings.IndexByte(line, '=')
	if i <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:i])
	val = strings.TrimSpace(line[i+1:])
	// Strip matching surrounding quotes.
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
	}
	return key, val, key != ""
}
