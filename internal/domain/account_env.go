package domain

// AccountEnvEntriesToMap converts a slice of AccountEnvEntry structs into a
// map of environment variable key-value pairs. Nil or empty slices return nil.
// In the case of duplicate keys, the last value in the slice wins.
func AccountEnvEntriesToMap(entries []AccountEnvEntry) map[string]string {
	if len(entries) == 0 {
		return nil
	}

	m := make(map[string]string, len(entries))
	for _, entry := range entries {
		m[entry.EnvKey] = entry.EnvValue
	}
	return m
}
