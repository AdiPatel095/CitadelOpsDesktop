package API

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"errors"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

const (
	// compressionThreshold is the smallest response or websocket message worth
	// compressing. Below it the CPU cost outweighs the saving on a 1-vCPU cell.
	compressionThreshold = 1024
	// compressionLevel favours CPU over ratio: JSON state still shrinks about 9x.
	compressionLevel = gzip.BestSpeed
)

var gzipWriters = sync.Pool{New: func() any {
	writer, _ := gzip.NewWriterLevel(nil, compressionLevel)
	return writer
}}

// compressResponses gzips compressible responses of at least compressionThreshold
// bytes for clients that accept it. Anything streamed or already encoded passes
// through untouched: server-sent events (text/event-stream, Cache-Control
// no-transform), websocket upgrades, ranges, HEAD, 1xx/204/304, and responses
// that already carry a Content-Encoding.
func compressResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !acceptsGzip(request) || request.Method == http.MethodHead ||
			strings.EqualFold(request.Header.Get("Upgrade"), "websocket") || request.Header.Get("Range") != "" {
			next.ServeHTTP(writer, request)
			return
		}
		compressed := &compressingWriter{ResponseWriter: writer}
		defer compressed.finish()
		next.ServeHTTP(compressed, request)
	})
}

func acceptsGzip(request *http.Request) bool {
	for _, header := range request.Header.Values("Accept-Encoding") {
		for _, part := range strings.Split(header, ",") {
			name, parameters, _ := strings.Cut(strings.TrimSpace(part), ";")
			if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
				continue
			}
			// "gzip;q=0" refuses gzip.
			if quality, found := strings.CutPrefix(strings.ReplaceAll(strings.TrimSpace(parameters), " ", ""), "q="); found {
				if value, err := strconv.ParseFloat(quality, 64); err == nil && value <= 0 {
					return false
				}
			}
			return true
		}
	}
	return false
}

type compressionMode int

const (
	compressionUndecided compressionMode = iota
	compressionBuffering
	compressionPassThrough
	compressionGzip
)

type compressingWriter struct {
	http.ResponseWriter
	mode       compressionMode
	status     int
	buffer     []byte
	gzip       *gzip.Writer
	headerSent bool
}

func (writer *compressingWriter) Unwrap() http.ResponseWriter { return writer.ResponseWriter }

func (writer *compressingWriter) WriteHeader(status int) {
	if writer.status != 0 || writer.headerSent {
		return
	}
	writer.status = status
	if status >= 100 && status < 200 {
		// Informational responses are forwarded as they are; the final status follows.
		writer.status = 0
		writer.ResponseWriter.WriteHeader(status)
		return
	}
	writer.decide()
	if writer.mode == compressionPassThrough {
		writer.sendHeader()
	}
}

// decide chooses between pass-through and buffering from the response headers.
func (writer *compressingWriter) decide() {
	if writer.mode != compressionUndecided {
		return
	}
	header := writer.Header()
	if !compressibleType(header.Get("Content-Type")) {
		writer.mode = compressionPassThrough
		return
	}
	// The response varies by encoding whether or not this one is compressed.
	header.Add("Vary", "Accept-Encoding")
	if writer.status == http.StatusNoContent || writer.status == http.StatusNotModified ||
		header.Get("Content-Encoding") != "" ||
		strings.Contains(strings.ToLower(header.Get("Cache-Control")), "no-transform") {
		writer.mode = compressionPassThrough
		return
	}
	if length, err := strconv.ParseInt(header.Get("Content-Length"), 10, 64); err == nil && length < compressionThreshold {
		writer.mode = compressionPassThrough
		return
	}
	writer.mode = compressionBuffering
}

func compressibleType(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch {
	case mediaType == "text/event-stream":
		return false
	case mediaType == "application/json", strings.HasSuffix(mediaType, "+json"),
		mediaType == "application/javascript", mediaType == "application/x-ndjson", mediaType == "image/svg+xml", mediaType == "application/xml",
		strings.HasPrefix(mediaType, "text/"):
		return true
	}
	return false
}

func (writer *compressingWriter) sendHeader() {
	if writer.headerSent {
		return
	}
	writer.headerSent = true
	status := writer.status
	if status == 0 {
		status = http.StatusOK
	}
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *compressingWriter) Write(data []byte) (int, error) {
	if writer.status == 0 && !writer.headerSent {
		writer.WriteHeader(http.StatusOK)
	}
	switch writer.mode {
	case compressionPassThrough:
		writer.sendHeader()
		return writer.ResponseWriter.Write(data)
	case compressionGzip:
		return writer.gzip.Write(data)
	}
	// Buffering: nothing is sent until the body reaches the threshold.
	writer.buffer = append(writer.buffer, data...)
	if len(writer.buffer) >= compressionThreshold {
		if err := writer.startGzip(); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}

func (writer *compressingWriter) startGzip() error {
	header := writer.Header()
	header.Set("Content-Encoding", "gzip")
	header.Del("Content-Length")
	// A strong ETag names the identity representation; the compressed one differs.
	if etag := header.Get("Etag"); etag != "" && !strings.HasPrefix(etag, "W/") {
		header.Set("Etag", "W/"+etag)
	}
	writer.sendHeader()
	writer.gzip = gzipWriters.Get().(*gzip.Writer)
	writer.gzip.Reset(writer.ResponseWriter)
	writer.mode = compressionGzip
	buffered := writer.buffer
	writer.buffer = nil
	_, err := writer.gzip.Write(buffered)
	return err
}

// Flush sends what has been written so far. A flushed response is being
// streamed, so a buffered one is committed to gzip (streaming, flushed
// incrementally) rather than held back.
func (writer *compressingWriter) Flush() {
	if writer.status == 0 && !writer.headerSent {
		writer.WriteHeader(http.StatusOK)
	}
	switch writer.mode {
	case compressionBuffering:
		if len(writer.buffer) > 0 {
			if writer.startGzip() != nil {
				return
			}
		} else {
			writer.mode = compressionPassThrough
			writer.sendHeader()
		}
	}
	if writer.mode == compressionGzip {
		_ = writer.gzip.Flush()
	}
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (writer *compressingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := writer.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("the underlying response writer does not support hijacking")
}

// finish flushes what remains: a short buffered body goes out uncompressed.
func (writer *compressingWriter) finish() {
	switch writer.mode {
	case compressionBuffering:
		writer.mode = compressionPassThrough
		writer.sendHeader()
		if len(writer.buffer) > 0 {
			_, _ = writer.ResponseWriter.Write(writer.buffer)
			writer.buffer = nil
		}
	case compressionGzip:
		_ = writer.gzip.Close()
		writer.gzip.Reset(nil)
		gzipWriters.Put(writer.gzip)
		writer.gzip = nil
	case compressionUndecided:
		// The handler wrote nothing: an empty body needs no encoding.
		if writer.status != 0 {
			writer.mode = compressionPassThrough
			writer.sendHeader()
		}
	}
}

// thresholdSocket writes JSON messages compressed only when they are at least
// compressionThreshold bytes and permessage-deflate was negotiated.
type thresholdSocket struct {
	*websocket.Conn
}

func (socket *thresholdSocket) WriteJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	socket.EnableWriteCompression(len(data) >= compressionThreshold)
	return socket.WriteMessage(websocket.TextMessage, data)
}

func newSocketCompression(connection *websocket.Conn) {
	_ = connection.SetCompressionLevel(flate.BestSpeed)
}
