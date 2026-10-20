package helpers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

const AuthBodyLimit = 4096

// DecodeAuthJSON bounds the entire body, including unknown fields and trailing
// data. Reading a bounded buffer first also rejects extra JSON documents before
// a handler can perform any credential operation.
func DecodeAuthJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, AuthBodyLimit)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return io.EOF
	}
	return json.Unmarshal(data, target)
}
