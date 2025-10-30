package prefetch

import (
	"context"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string {
	return &s
}

func cmdEntry(cmd string) *osdd.PrefetchEntry {
	return osdd.PrefetchEntry_builder{Cmd: strPtr(cmd)}.Build()
}

func prefetchWith(entries ...*osdd.PrefetchEntry) *osdd.Prefetch {
	return osdd.Prefetch_builder{Entries: entries}.Build()
}

func assertResult(t *testing.T, result map[string]*osdd.FetchedData, expected map[string]string) {
	t.Helper()
	require.NotNil(t, result)
	assert.Len(t, result, len(expected))
	for id, data := range expected {
		assert.Contains(t, result, id)
		assert.Equal(t, data, result[id].GetData())
	}
}

func TestProcessor_Process(t *testing.T) {
	tests := []struct {
		name     string
		prefetch *osdd.Prefetch
		wantErr  string
		want     map[string]string
	}{
		{
			name: "nil prefetch",
		},
		{
			name:     "nil entries",
			prefetch: osdd.Prefetch_builder{}.Build(),
		},
		{
			name:     "empty entries",
			prefetch: prefetchWith(),
		},
		{
			name:     "single entry",
			prefetch: prefetchWith(cmdEntry(`echo '{"data": [{"id": "test-id", "data": "test data"}]}'`)),
			want:     map[string]string{"test-id": "test data"},
		},
		{
			name: "multiple entries",
			prefetch: prefetchWith(
				cmdEntry(`echo '{"data": [{"id": "id-1", "data": "data 1"}]}'`),
				cmdEntry(`echo '{"data": [{"id": "id-2", "data": "data 2"}]}'`),
			),
			want: map[string]string{"id-1": "data 1", "id-2": "data 2"},
		},
		{
			name:     "multiple data in single entry",
			prefetch: prefetchWith(cmdEntry(`echo '{"data": [{"id": "item-1", "data": "first"}, {"id": "item-2", "data": "second"}]}'`)),
			want:     map[string]string{"item-1": "first", "item-2": "second"},
		},
		{
			name:     "nil entry",
			prefetch: prefetchWith(nil),
			wantErr:  "prefetch entry at index 0 is nil",
		},
		{
			name:     "invalid JSON",
			prefetch: prefetchWith(cmdEntry(`echo 'not valid json'`)),
			wantErr:  "failed to unmarshal prefetch result",
		},
		{
			name:     "empty data array",
			prefetch: prefetchWith(cmdEntry(`echo '{"data": []}'`)),
		},
		{
			name: "partial failure",
			prefetch: prefetchWith(
				cmdEntry(`echo '{"data": [{"id": "id-1", "data": "data 1"}]}'`),
				cmdEntry("exit 1"),
			),
			wantErr: "failed to process entry at index 1",
		},
		{
			name:     "complex JSON",
			prefetch: prefetchWith(cmdEntry(`echo '{"data": [{"id": "config-1", "data": "{\"key\": \"value\"}"}]}'`)),
			want:     map[string]string{"config-1": `{"key": "value"}`},
		},
		{
			name: "duplicate IDs - last wins",
			prefetch: prefetchWith(
				cmdEntry(`echo '{"data": [{"id": "same-id", "data": "first"}]}'`),
				cmdEntry(`echo '{"data": [{"id": "same-id", "data": "second"}]}'`),
			),
			want: map[string]string{"same-id": "second"},
		},
		{
			name:     "multiline output",
			prefetch: prefetchWith(cmdEntry(`printf '{"data": [{"id": "multi", "data": "line1\\nline2"}]}'`)),
			want:     map[string]string{"multi": "line1\nline2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &Processor{}
			result, err := p.Process(context.Background(), tt.prefetch)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.want != nil {
				assertResult(t, result, tt.want)
			} else {
				assert.Empty(t, result)
			}
		})
	}
}

func TestProcessor_ProcessEntry(t *testing.T) {
	tests := []struct {
		name    string
		entry   *osdd.PrefetchEntry
		wantErr string
		want    string
	}{
		{
			name:    "empty command",
			entry:   cmdEntry(""),
			wantErr: "cmd cannot be empty",
		},
		{
			name:    "failed command",
			entry:   cmdEntry("exit 1"),
			wantErr: "command execution failed",
		},
		{
			name:    "no type set",
			entry:   &osdd.PrefetchEntry{},
			wantErr: "unknown or unset prefetch entry type",
		},
		{
			name:  "successful command",
			entry: cmdEntry(`echo 'test output'`),
			want:  "test output\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &Processor{}
			data, err := p.processEntry(context.Background(), tt.entry)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, data)
		})
	}
}

func TestProcessor_Process_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := &Processor{}
	_, err := p.Process(ctx, prefetchWith(cmdEntry(`sleep 10`)))
	assert.Error(t, err)
}
