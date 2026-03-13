package utils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchURL(t *testing.T) {
	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer successServer.Close()

	notFoundServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer notFoundServer.Close()

	serverErrorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer serverErrorServer.Close()

	tests := []struct {
		name    string
		url     string
		want    []byte
		wantErr string
	}{
		{
			name: "success",
			url:  successServer.URL,
			want: []byte("hello world"),
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: "url cannot be empty",
		},
		{
			name:    "invalid scheme ftp",
			url:     "ftp://example.com/file",
			wantErr: "url scheme must be http or https",
		},
		{
			name:    "no scheme",
			url:     "just-a-string",
			wantErr: "url scheme must be http or https",
		},
		{
			name:    "404 response",
			url:     notFoundServer.URL + "/missing",
			wantErr: "returned status 404",
		},
		{
			name:    "500 response",
			url:     serverErrorServer.URL + "/error",
			wantErr: "returned status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := FetchURL(context.Background(), tt.url)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, body)
		})
	}
}

func TestValidateURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{name: "valid http", url: "http://example.com/file"},
		{name: "valid https", url: "https://example.com/file"},
		{name: "empty", url: "", wantErr: "url cannot be empty"},
		{name: "ftp scheme", url: "ftp://example.com", wantErr: "url scheme must be http or https"},
		{name: "no scheme", url: "just-a-string", wantErr: "url scheme must be http or https"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateURL(tt.url)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFetchURL_BinaryContent(t *testing.T) {
	t.Parallel()

	// Binary data with non-UTF-8 bytes (PNG magic bytes + extras)
	binaryData := []byte{0x00, 0x01, 0xFF, 0xFE, 0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(binaryData)
	}))
	defer server.Close()

	body, err := FetchURL(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, binaryData, body)
}

func TestFetchURL_ContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := FetchURL(ctx, server.URL)
	require.Error(t, err)
}
