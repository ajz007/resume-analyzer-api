package analyses

import (
	"fmt"
	"strings"
)

type ValidationSeverity string

const (
	ValidationFatal       ValidationSeverity = "fatal"
	ValidationRecoverable ValidationSeverity = "recoverable"
	ValidationWarning     ValidationSeverity = "warning"
)

type ValidationIssue struct {
	Path     string             `json:"path"`
	Message  string             `json:"message"`
	Severity ValidationSeverity `json:"severity"`
}

type ValidationIssuesError struct {
	Issues []ValidationIssue
}

func (e ValidationIssuesError) Error() string {
	if len(e.Issues) == 0 {
		return "validation failed"
	}
	first := e.Issues[0]
	if strings.TrimSpace(first.Path) == "" {
		return first.Message
	}
	return fmt.Sprintf("%s: %s", first.Path, first.Message)
}

func countValidationIssuesBySeverity(issues []ValidationIssue) map[ValidationSeverity]int {
	counts := map[ValidationSeverity]int{
		ValidationFatal:       0,
		ValidationRecoverable: 0,
		ValidationWarning:     0,
	}
	for _, issue := range issues {
		counts[issue.Severity]++
	}
	return counts
}

func fatalValidationIssues(issues []ValidationIssue) []ValidationIssue {
	fatal := make([]ValidationIssue, 0)
	for _, issue := range issues {
		if issue.Severity == ValidationFatal {
			fatal = append(fatal, issue)
		}
	}
	return fatal
}

func appendFatalValidationIssue(issues []ValidationIssue, path string, err error) []ValidationIssue {
	if err == nil {
		return issues
	}
	return append(issues, ValidationIssue{
		Path:     path,
		Message:  err.Error(),
		Severity: ValidationFatal,
	})
}
