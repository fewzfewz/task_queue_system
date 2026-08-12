package webhooks

import "strings"

// NormalizeEvents converts legacy event names (job.completed) to canonical form (completed).
func NormalizeEvents(events []string) []string {
	if len(events) == 0 {
		return []string{"completed", "failed"}
	}
	out := make([]string, 0, len(events))
	for _, e := range events {
		e = strings.TrimSpace(e)
		e = strings.TrimPrefix(e, "job.")
		if e != "" {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return []string{"completed", "failed"}
	}
	return out
}

// NormalizeEvent normalizes a single event string.
func NormalizeEvent(event string) string {
	event = strings.TrimSpace(event)
	return strings.TrimPrefix(event, "job.")
}
