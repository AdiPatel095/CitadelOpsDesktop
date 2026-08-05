package Runtime

import (
	"fmt"
	"net/url"
	"strings"
)

// SQLiteFileDSN returns a SQLite file URI for an absolute native path.
// Windows drive paths need a leading slash so the drive is parsed as part of
// the URI path instead of as its authority.
func SQLiteFileDSN(filename string, pragmas ...string) (string, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "", fmt.Errorf("sqlite database path is required")
	}

	databaseURL := url.URL{Scheme: "file"}
	switch {
	case isWindowsDrivePath(filename):
		databaseURL.Path = "/" + strings.ReplaceAll(filename, `\`, "/")
	case strings.HasPrefix(filename, `\\`) || strings.HasPrefix(filename, "//"):
		normalized := strings.TrimPrefix(strings.ReplaceAll(filename, `\`, "/"), "//")
		host, path, found := strings.Cut(normalized, "/")
		if !found || strings.TrimSpace(host) == "" || strings.Trim(path, "/") == "" {
			return "", fmt.Errorf("sqlite UNC database path must include a server and share")
		}
		databaseURL.Host = host
		databaseURL.Path = "/" + path
	case strings.HasPrefix(filename, "/"):
		databaseURL.Path = filename
	default:
		return "", fmt.Errorf("sqlite database path must be absolute: %q", filename)
	}

	parameters := databaseURL.Query()
	for _, pragma := range pragmas {
		pragma = strings.TrimSpace(pragma)
		if pragma != "" {
			parameters.Add("_pragma", pragma)
		}
	}
	databaseURL.RawQuery = parameters.Encode()
	return databaseURL.String(), nil
}

func isWindowsDrivePath(filename string) bool {
	if len(filename) < 3 || filename[1] != ':' || (filename[2] != '\\' && filename[2] != '/') {
		return false
	}
	drive := filename[0]
	return (drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')
}
