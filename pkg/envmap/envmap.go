package envmap

import (
	"fmt"
	"strings"
)

func EnvToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, s := range env {
		if s != "" {
			kv := strings.SplitN(s, "=", 2)
			if len(kv) == 2 {
				m[kv[0]] = kv[1]
			} else {
				m[kv[0]] = ""
			}
		}
	}
	return m
}

func MapToEnv(m map[string]string) []string {
	s := make([]string, 0, len(m))
	for k, v := range m {
		s = append(s, fmt.Sprintf("%s=%s", k, v))
	}
	return s
}
