# Core libraries supporting OpenSDD

## Integration Tests

Integration tests require credentials and are skipped by default (they run when `-short` is not set).

### Configuration

Create `~/.config/osdd/.env.integ-test` with the required variables:

```env
# Jira
OSDD_TEST_JIRA_ORG=devplan
OSDD_TEST_JIRA_TOKEN=user@example.com:your-api-token
OSDD_TEST_JIRA_PROJECT=TesCP
OSDD_TEST_JIRA_ISSUE=TES-311

# Linear
OSDD_TEST_LINEAR_TOKEN=lin_api_xxxxxxxxxxxx
OSDD_TEST_LINEAR_TEAM=ENG
OSDD_TEST_LINEAR_ISSUE=ENG-10
```

The `OSDD_TEST_JIRA_TOKEN` value should be in `email:api-token` format for Jira Cloud personal access tokens (PAT). You can generate an API token at https://id.atlassian.com/manage-profile/security/api-tokens.

The `OSDD_TEST_LINEAR_TOKEN` is a Linear API key from https://linear.app/settings/api.

Variables can also be set as environment variables directly; the file is a fallback.

### Running

```bash
# Unit tests only (fast)
go test ./... -short -count=1

# All tests including integration
go test ./... -count=1

# Specific integration tests
go test ./core/utils/ -run Integration -count=1 -v
go test ./core/generators/ -run Integration -count=1 -v
```

### Golden File Tests

The `TestContext_Golden_JiraDevplan` test compares materialized Jira output against stored golden files. It only runs when `OSDD_TEST_JIRA_ORG=devplan`.

To regenerate golden files after Jira data changes:

```bash
go test ./core/generators/ -run Golden_JiraDevplan -count=1 -update -v
```

Golden files are stored in `core/generators/testdata/jira_devplan_golden/`.
