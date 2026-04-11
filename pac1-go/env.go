package main

import (
	"bufio"
	"os"
	"strings"
)

// envOr returns $name or fallback when unset/empty.
func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv reads KEY=VALUE lines out of ./.env (if present) into the
// process environment without overriding variables that are already set.
// Kept deliberately tiny — no quoting, no export prefix, no multiline.
// Matches the hackathon "copy-paste this line into .env" instructions.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		// Strip one pair of surrounding quotes if present.
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		if _, alreadySet := os.LookupEnv(k); alreadySet {
			continue
		}
		_ = os.Setenv(k, v)
	}
}
