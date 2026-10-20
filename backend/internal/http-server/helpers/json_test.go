package helpers_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	"github.com/stretchr/testify/require"
)

type countedBody struct {
	io.Reader
	read int
}

func (b *countedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (*countedBody) Close() error { return nil }

func TestAuthJSONBoundsCompleteBody(t *testing.T) {
	for _, body := range []string{`{"unknown":"` + strings.Repeat("x", 8<<20) + `"}`, `{}` + strings.Repeat(" ", 8<<20)} {
		reader := &countedBody{Reader: strings.NewReader(body)}
		r := httptest.NewRequest("POST", "/login", reader)
		r.ContentLength = -1 // Chunked/unknown-length bodies must be bounded too.
		var value map[string]any
		err := helpers.DecodeAuthJSON(httptest.NewRecorder(), r, &value)
		var tooLarge *http.MaxBytesError
		require.True(t, errors.As(err, &tooLarge))
		require.LessOrEqual(t, reader.read, helpers.AuthBodyLimit+1)
	}
}

func TestAuthJSONRejectsTrailingDocumentsAndAcceptsBoundary(t *testing.T) {
	var value map[string]any
	for _, body := range []string{`{} {}`, `{} trailing`} {
		r := httptest.NewRequest("POST", "/login", strings.NewReader(body))
		require.Error(t, helpers.DecodeAuthJSON(httptest.NewRecorder(), r, &value))
	}
	r := httptest.NewRequest("POST", "/login", strings.NewReader(`{}`+strings.Repeat(" ", helpers.AuthBodyLimit-2)))
	require.NoError(t, helpers.DecodeAuthJSON(httptest.NewRecorder(), r, &value))
}
