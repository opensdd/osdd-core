package utils

// ExportJiraBaseURL returns the current Jira base URL (for cross-package tests).
func ExportJiraBaseURL() string { return jiraBaseURL }

// SetJiraBaseURL overrides the Jira base URL (for cross-package tests).
func SetJiraBaseURL(url string) { jiraBaseURL = url }

// ExportLinearBaseURL returns the current Linear base URL (for cross-package tests).
func ExportLinearBaseURL() string { return linearBaseURL }

// SetLinearBaseURL overrides the Linear base URL (for cross-package tests).
func SetLinearBaseURL(url string) { linearBaseURL = url }

// ExportGitHubAPIBaseURL returns the current GitHub API base URL (for cross-package tests).
func ExportGitHubAPIBaseURL() string { return githubAPIBaseURL }

// SetGitHubAPIBaseURL overrides the GitHub API base URL (for cross-package tests).
func SetGitHubAPIBaseURL(url string) { githubAPIBaseURL = url }

// ExportBitbucketAPIBaseURL returns the current Bitbucket API base URL (for cross-package tests).
func ExportBitbucketAPIBaseURL() string { return bitbucketAPIBaseURL }

// SetBitbucketAPIBaseURL overrides the Bitbucket API base URL (for cross-package tests).
func SetBitbucketAPIBaseURL(url string) { bitbucketAPIBaseURL = url }
