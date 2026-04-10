package GameParser

// CommandType returns the game protocol command (3rd field after splitting the frame on "%").
func CommandType(parts []string) (cmd string, ok bool) {
	if len(parts) <= 2 {
		return "", false
	}
	return parts[2], true
}

// Payload returns the JSON payload segment at index 5 (game frames: ...%cmd%...%payload).
func Payload(parts []string) (payload string, ok bool) {
	if len(parts) <= 5 {
		return "", false
	}
	return parts[5], true
}
