package aclsnapshot

import (
	"net/http"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// Compression preserves content negotiation on bodyless conditional responses,
// where the gzip middleware otherwise removes Vary along with Content-Encoding.
func Compression() gin.HandlerFunc {
	compress := gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/metrics"}))
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/v2/acl/snapshot" {
			c.Writer = encodingWriter{c.Writer}
		}
		compress(c)
	}
}

type encodingWriter struct{ gin.ResponseWriter }

func (w encodingWriter) Write(data []byte) (int, error) {
	varyEncoding(w.Header())
	return w.ResponseWriter.Write(data)
}

func (w encodingWriter) WriteHeaderNow() {
	varyEncoding(w.Header())
	w.ResponseWriter.WriteHeaderNow()
}

func varyEncoding(header http.Header) {
	for _, value := range header.Values("Vary") {
		for _, field := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(field), "Accept-Encoding") || strings.TrimSpace(field) == "*" {
				return
			}
		}
	}
	header.Add("Vary", "Accept-Encoding")
}
