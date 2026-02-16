package utils

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildGitCloneURL(t *testing.T) {
	tests := []struct {
		name    string
		repo    *osdd.GitRepository
		envVars map[string]string
		want    string
		wantErr string
	}{
		{
			name: "github provider explicit",
			repo: osdd.GitRepository_builder{FullName: "owner/repo", Provider: "github"}.Build(),
			want: "https://github.com/owner/repo.git",
		},
		{
			name: "empty provider defaults to github",
			repo: osdd.GitRepository_builder{FullName: "owner/repo"}.Build(),
			want: "https://github.com/owner/repo.git",
		},
		{
			name: "bitbucket provider",
			repo: osdd.GitRepository_builder{FullName: "owner/repo", Provider: "bitbucket"}.Build(),
			want: "https://bitbucket.org/owner/repo.git",
		},
		{
			name:    "github with auth token",
			repo:    osdd.GitRepository_builder{FullName: "owner/repo", Provider: "github", AuthTokenEnvVar: strPtr("TEST_GIT_TOKEN")}.Build(),
			envVars: map[string]string{"TEST_GIT_TOKEN": "mytoken123"},
			want:    "https://x-access-token:mytoken123@github.com/owner/repo.git",
		},
		{
			name:    "bitbucket with auth token",
			repo:    osdd.GitRepository_builder{FullName: "owner/repo", Provider: "bitbucket", AuthTokenEnvVar: strPtr("TEST_BB_TOKEN")}.Build(),
			envVars: map[string]string{"TEST_BB_TOKEN": "bbtoken456"},
			want:    "https://x-token-auth:bbtoken456@bitbucket.org/owner/repo.git",
		},
		{
			name:    "auth env var configured but empty",
			repo:    osdd.GitRepository_builder{FullName: "owner/repo", Provider: "github", AuthTokenEnvVar: strPtr("TEST_EMPTY_TOKEN")}.Build(),
			envVars: map[string]string{"TEST_EMPTY_TOKEN": ""},
			want:    "https://github.com/owner/repo.git",
		},
		{
			name: "auth env var configured but not set",
			repo: osdd.GitRepository_builder{FullName: "owner/repo", Provider: "github", AuthTokenEnvVar: strPtr("TEST_UNSET_TOKEN")}.Build(),
			want: "https://github.com/owner/repo.git",
		},
		{
			name:    "nil repo",
			repo:    nil,
			wantErr: "git repository cannot be nil",
		},
		{
			name:    "empty full name",
			repo:    osdd.GitRepository_builder{Provider: "github"}.Build(),
			wantErr: "git repository full name cannot be empty",
		},
		{
			name:    "unknown provider",
			repo:    osdd.GitRepository_builder{FullName: "owner/repo", Provider: "gitlab"}.Build(),
			wantErr: "unsupported git provider: gitlab",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env vars for this test
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			url, err := BuildGitCloneURL(tt.repo)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, url)
		})
	}
}

func TestCloneGitRepo(t *testing.T) {
	t.Run("nil repo", func(t *testing.T) {
		err := CloneGitRepo(context.Background(), nil, t.TempDir())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "git repository cannot be nil")
	})

	t.Run("invalid url causes clone failure", func(t *testing.T) {
		repo := osdd.GitRepository_builder{FullName: "nonexistent/repo-that-does-not-exist-anywhere-99999", Provider: "github"}.Build()
		dest := filepath.Join(t.TempDir(), "clone-target")
		err := CloneGitRepo(context.Background(), repo, dest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "git clone failed")
	})

	// NOTE: The CloneGitRepo success path is tested via integration tests
	// (TestContext_IntegrationTest_GitRepoSource) which perform a real clone
	// of a public GitHub repository. Unit tests here cover error paths only
	// because BuildGitCloneURL always constructs https:// URLs, making it
	// impossible to test with local bare repos in a unit test.
}
