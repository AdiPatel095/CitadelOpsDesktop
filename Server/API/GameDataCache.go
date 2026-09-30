package API

import (
	"net/http"
	"strings"

	"CitadelDesktop/Server/GameData"
)

const immutableCatalogCacheControl = "public, max-age=31536000, immutable"

// collectionCacheHeaders sets the HTTP caching of a game-data collection and
// reports whether the request was answered with 304 (nothing more to write).
//
// A collection's content is a function of the game-data source digest and the
// collection name, so the digest is the cache key:
//   - `?digest=<current digest>`: the URL names this exact content, so it is
//     immutable and cached for a year. The locale is part of the URL as well, so
//     languages never share an entry.
//   - `?digest=<another digest>`: the caller's manifest is stale. The current
//     content is still served, but marked no-store: it must never be cached under
//     a URL that names different content.
//   - no digest (older callers): the response is revalidated with its ETag.
//
// A source without a digest (tests, an unloaded catalog) keeps the previous,
// header-less behavior.
func collectionCacheHeaders(writer http.ResponseWriter, request *http.Request, digest string, name string, locale *GameData.LocaleResolution) bool {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return false
	}
	header := writer.Header()
	requested := strings.TrimSpace(request.URL.Query().Get("digest"))
	if requested != "" && requested != digest {
		header.Set("Cache-Control", "no-store")
		header.Set("X-Catalog-Digest", digest)
		return false
	}
	resolved := ""
	if locale != nil {
		resolved = locale.ResolvedLocale
	}
	etag := `"` + digest + "-" + name + "-" + resolved + `"`
	header.Set("ETag", etag)
	if requested == digest {
		header.Set("Cache-Control", immutableCatalogCacheControl)
	} else {
		header.Set("Cache-Control", "no-cache")
	}
	if strongETagMatches(request.Header.Get("If-None-Match"), etag) {
		writer.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

// strongETagMatches reports whether an If-None-Match header lists etag (or is
// "*"). Weak validators are compared by their opaque tag, as RFC 9110 requires
// for If-None-Match.
func strongETagMatches(header string, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, candidate := range strings.Split(header, ",") {
		if strings.TrimPrefix(strings.TrimSpace(candidate), "W/") == want {
			return true
		}
	}
	return false
}
