package utils

// ExportJiraBaseURL returns the current Jira base URL (for cross-package tests).
func ExportJiraBaseURL() string { return jiraBaseURL }

// SetJiraBaseURL overrides the Jira base URL (for cross-package tests).
func SetJiraBaseURL(url string) { jiraBaseURL = url }

// ExportLinearBaseURL returns the current Linear base URL (for cross-package tests).
func ExportLinearBaseURL() string { return linearBaseURL }

// SetLinearBaseURL overrides the Linear base URL (for cross-package tests).
func SetLinearBaseURL(url string) { linearBaseURL = url }
