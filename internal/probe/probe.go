package probe

import (
	"fmt"
	"regexp"
	"strings"
)

// A release cycle name is an endoflife.date API path segment, so it must be a
// plain slug. This also blocks path traversal via the query string.
var slugRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// parseReleases turns the releases query parameter into a fetch plan.
// all=true means fetch every cycle; otherwise releases holds the cycle names.
// shortcut: "all" is a reserved value, rename the param if a cycle is ever named that.
func parseReleases(param string) (releases []string, all bool, err error) {
	param = strings.TrimSpace(param)
	if param == "" {
		return []string{"latest"}, false, nil
	}

	parts := strings.Split(param, ",")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false, fmt.Errorf("empty release cycle in %q", param)
		}
		parts[i] = part
	}

	if len(parts) > 1 {
		for _, part := range parts {
			if part == "all" {
				return nil, false, fmt.Errorf(`"all" cannot be combined with release cycles: %q`, param)
			}
		}
	}
	if parts[0] == "all" {
		return nil, true, nil
	}

	// Dedupe while keeping the requested order: duplicate cycles would emit
	// identical series and make Prometheus fail the whole scrape.
	seen := make(map[string]struct{}, len(parts))
	unique := make([]string, 0, len(parts))
	for _, part := range parts {
		if !slugRe.MatchString(part) {
			return nil, false, fmt.Errorf("invalid release cycle name %q", part)
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		unique = append(unique, part)
	}

	return unique, false, nil
}
