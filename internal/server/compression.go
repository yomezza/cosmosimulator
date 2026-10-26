package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

type compressedResponse struct {
	http.ResponseWriter
	stream io.Writer
}

func (w compressedResponse) Write(data []byte) (int, error) { return w.stream.Write(data) }

// Static world/catalogue responses are large; browsers and standard HTTP clients
// negotiate and decode gzip without a frontend dependency or a second service.
func gzipAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/") && acceptsGzip(r.Header.Get("Accept-Encoding")) {
			stream, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
			defer stream.Close()
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Add("Vary", "Accept-Encoding")
			next.ServeHTTP(compressedResponse{w, stream}, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func acceptsGzip(header string) bool {
	for _, encoding := range strings.Split(header, ",") {
		parts := strings.Split(encoding, ";")
		if strings.TrimSpace(parts[0]) != "gzip" {
			continue
		}
		disabled := false
		for _, p := range parts[1:] {
			value := strings.TrimSpace(p)
			if value == "q=0" || value == "q=0.0" || value == "q=0.00" || value == "q=0.000" {
				disabled = true
			}
		}
		if !disabled {
			return true
		}
	}
	return false
}
