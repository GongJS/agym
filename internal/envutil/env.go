package envutil

import "strings"

// Replace returns a copy of environment with any keys in values overridden or appended.
func Replace(environment []string, values map[string]string) []string {
	result := make([]string, 0, len(environment)+len(values))
	for _, item := range environment {
		name, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, replace := values[name]; replace {
			continue
		}
		result = append(result, item)
	}
	for name, value := range values {
		result = append(result, name+"="+value)
	}
	return result
}
