package utils

// IssueSummary holds the minimal identifier and title for an issue.
type IssueSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// IssuesResult is the structured output from FetchJiraIssues / FetchLinearIssues.
// Summary contains one entry per issue (id + title) for the index file.
// Issues maps each issue ID to its full raw JSON content.
type IssuesResult struct {
	Summary []IssueSummary
	Issues  map[string]string // issueID → full JSON content
}
