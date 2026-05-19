package cli

// stripAccountFlag scans args for --account <value> or --account=<value>,
// strips the first match, and returns the extracted account name and the
// remaining argument slice. If no --account is present, it returns ("", args).
//
// The scan stops at "--" (which DisableFlagParsing passes through as a literal
// token). Any --account that appears after a "--" separator is left untouched.
//
// When multiple --account flags are present, the first match wins and all
// subsequent occurrences are preserved in the remaining slice.
//
// Malformed cases — "--account" as the last token with no following value, or
// "--account=" with an empty value — return ("", args).
func stripAccountFlag(args []string) (accountName string, remaining []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// "--" is the end-of-options sentinel: stop scanning.
		if arg == "--" {
			return "", args
		}

		// "--account=<value>" form.
		if len(arg) > len("--account=") && arg[:len("--account=")] == "--account=" {
			value := arg[len("--account="):]
			if value == "" {
				// "--account=" with no value: treat as absent.
				return "", args
			}
			remaining = make([]string, 0, len(args)-1)
			remaining = append(remaining, args[:i]...)
			remaining = append(remaining, args[i+1:]...)
			return value, remaining
		}

		// "--account <value>" (space-separated) form.
		if arg == "--account" {
			next := i + 1
			if next >= len(args) {
				// "--account" with no following token: malformed; treat as absent.
				return "", args
			}
			value := args[next]
			if value == "" {
				return "", args
			}
			remaining = make([]string, 0, len(args)-2)
			remaining = append(remaining, args[:i]...)
			remaining = append(remaining, args[next+1:]...)
			return value, remaining
		}
	}
	return "", args
}
