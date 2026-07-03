package modelv1

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

var userInputSeverities = map[string]bool{
	"required": true,
	"optional": true,
}

var tailoringChangeTypes = map[string]bool{
	"rewrite":   true,
	"add":       true,
	"remove":    true,
	"reorder":   true,
	"no_change": true,
}

var tailoringRisks = map[string]bool{
	"safe":                    true,
	"needs_user_confirmation": true,
	"unsafe":                  true,
}

var tailoringSuggestionTypes = map[string]bool{
	"safe_rewrite":        true,
	"needs_confirmation":  true,
	"missing_requirement": true,
	"sample_example":      true,
}

// ValidateResumeGenerationResponse returns hard validation errors for the
// future AI generation response contract.
func ValidateResumeGenerationResponse(response ResumeGenerationResponse) []ValidationError {
	var errs []ValidationError
	errs = appendPrefixedErrors(errs, "resume", ValidateStructure(response.Resume))

	for i, item := range response.RequiresUserInput {
		prefix := fmt.Sprintf("requiresUserInput[%d]", i)
		errs = appendRequiredStringError(errs, prefix+".field", item.Field)
		errs = appendRequiredStringError(errs, prefix+".message", item.Message)
		errs = appendRequiredEnumError(errs, prefix+".severity", item.Severity, userInputSeverities)
		errs = appendMaxLengthError(errs, prefix+".field", item.Field, 200)
		errs = appendMaxLengthError(errs, prefix+".message", item.Message, 500)
	}
	for i, assumption := range response.Assumptions {
		field := fmt.Sprintf("assumptions[%d].message", i)
		errs = appendRequiredStringError(errs, field, assumption.Message)
		errs = appendMaxLengthError(errs, field, assumption.Message, 500)
	}
	for i, warning := range response.Warnings {
		field := fmt.Sprintf("warnings[%d].message", i)
		errs = appendRequiredStringError(errs, field, warning.Message)
		errs = appendMaxLengthError(errs, field, warning.Message, 500)
	}

	return errs
}

// ValidateResumeTailoringResponse returns hard validation errors for the future
// AI tailoring response contract.
func ValidateResumeTailoringResponse(response ResumeTailoringResponse) []ValidationError {
	var errs []ValidationError
	errs = appendPrefixedErrors(errs, "tailoredResume", ValidateStructure(response.TailoredResume))

	for i, change := range response.Changes {
		prefix := fmt.Sprintf("changes[%d]", i)
		errs = appendRequiredEnumError(errs, prefix+".section", change.Section, knownSectionKeys)
		errs = appendRequiredStringError(errs, prefix+".itemId", change.ItemID)
		errs = appendRequiredEnumError(errs, prefix+".changeType", change.ChangeType, tailoringChangeTypes)
		errs = appendChangeContentErrors(errs, prefix, change)
		errs = appendRequiredStringError(errs, prefix+".reason", change.Reason)
		errs = appendRequiredEnumError(errs, prefix+".risk", change.Risk, tailoringRisks)
		errs = appendMaxLengthError(errs, prefix+".itemId", change.ItemID, 120)
		errs = appendMaxLengthError(errs, prefix+".before", change.Before, 1000)
		errs = appendMaxLengthError(errs, prefix+".after", change.After, 1000)
		errs = appendMaxLengthError(errs, prefix+".reason", change.Reason, 700)
	}
	for i, requirement := range response.MissingRequirements {
		requirementField := fmt.Sprintf("missingRequirements[%d].requirement", i)
		messageField := fmt.Sprintf("missingRequirements[%d].message", i)
		exampleField := fmt.Sprintf("missingRequirements[%d].example", i)
		riskField := fmt.Sprintf("missingRequirements[%d].risk", i)
		errs = appendRequiredStringError(errs, requirementField, requirement.Requirement)
		errs = appendRequiredStringError(errs, messageField, requirement.Message)
		errs = appendRequiredStringError(errs, exampleField, requirement.Example)
		errs = appendRequiredEnumError(errs, riskField, requirement.Risk, tailoringRisks)
		errs = appendMaxLengthError(errs, requirementField, requirement.Requirement, 500)
		errs = appendMaxLengthError(errs, messageField, requirement.Message, 700)
		errs = appendMaxLengthError(errs, exampleField, requirement.Example, 700)
		errs = appendMaxLengthError(errs, fmt.Sprintf("missingRequirements[%d].recommendation", i), requirement.Recommendation, 700)
	}
	for i, suggestion := range response.Suggestions {
		prefix := fmt.Sprintf("suggestions[%d]", i)
		errs = appendRequiredEnumError(errs, prefix+".type", suggestion.Type, tailoringSuggestionTypes)
		errs = appendRequiredStringError(errs, prefix+".message", suggestion.Message)
		errs = appendMaxLengthError(errs, prefix+".section", suggestion.Section, 120)
		errs = appendMaxLengthError(errs, prefix+".itemId", suggestion.ItemID, 120)
		errs = appendMaxLengthError(errs, prefix+".requirement", suggestion.Requirement, 500)
		errs = appendMaxLengthError(errs, prefix+".message", suggestion.Message, 700)
		errs = appendMaxLengthError(errs, prefix+".example", suggestion.Example, 700)
		errs = appendMaxLengthError(errs, prefix+".before", suggestion.Before, 1000)
		errs = appendMaxLengthError(errs, prefix+".after", suggestion.After, 1000)
		errs = appendMaxLengthError(errs, prefix+".reason", suggestion.Reason, 700)
		if strings.TrimSpace(suggestion.Risk) != "" {
			errs = appendRequiredEnumError(errs, prefix+".risk", suggestion.Risk, tailoringRisks)
		}
	}
	for i, warning := range response.Warnings {
		field := fmt.Sprintf("warnings[%d].message", i)
		errs = appendRequiredStringError(errs, field, warning.Message)
		errs = appendMaxLengthError(errs, field, warning.Message, 500)
	}

	return errs
}

func appendChangeContentErrors(errs []ValidationError, prefix string, change TailoringChange) []ValidationError {
	switch change.ChangeType {
	case "rewrite":
		errs = appendRequiredStringError(errs, prefix+".before", change.Before)
		errs = appendRequiredStringError(errs, prefix+".after", change.After)
	case "add":
		errs = appendRequiredStringError(errs, prefix+".after", change.After)
	case "remove":
		errs = appendRequiredStringError(errs, prefix+".before", change.Before)
	case "reorder", "no_change":
		return errs
	default:
		return errs
	}
	return errs
}

func appendPrefixedErrors(errs []ValidationError, prefix string, nested []ValidationError) []ValidationError {
	for _, err := range nested {
		err.Field = prefix + "." + err.Field
		errs = append(errs, err)
	}
	return errs
}

func appendRequiredStringError(errs []ValidationError, field, value string) []ValidationError {
	if strings.TrimSpace(value) == "" {
		return append(errs, ValidationError{Field: field, Message: "is required"})
	}
	return errs
}

func appendRequiredEnumError(errs []ValidationError, field, value string, allowed map[string]bool) []ValidationError {
	if strings.TrimSpace(value) == "" {
		return append(errs, ValidationError{Field: field, Message: "is required"})
	}
	if !allowed[value] {
		return append(errs, ValidationError{Field: field, Message: "has an invalid value"})
	}
	return errs
}

func appendMaxLengthError(errs []ValidationError, field, value string, max int) []ValidationError {
	if utf8.RuneCountInString(value) > max {
		return append(errs, ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be %d characters or fewer", max),
		})
	}
	return errs
}
