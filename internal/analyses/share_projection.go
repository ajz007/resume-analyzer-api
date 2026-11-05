package analyses

// publicShareResult keeps the analysis and proposed wording, but removes original
// resume passages and source anchors from both new and previously created shares.
// The stored result remains available through the authenticated owner route.
func publicShareResult(result map[string]any) map[string]any {
	if result == nil {
		return nil
	}
	out := make(map[string]any, len(result))
	for key, value := range result {
		switch key {
		case "report", "bulletRewrites":
			// Legacy rewrites bundle the original passage with the suggestion.
		default:
			out[key] = value
		}
	}
	report, ok := result["report"].(map[string]any)
	if !ok {
		return out
	}
	public := publicFields(report, "schemaVersion", "scope", "priorities", "questions", "outcomes", "interviewPreparation")
	if edits, ok := report["edits"].([]any); ok {
		public["edits"] = publicRows(edits, func(edit map[string]any) map[string]any {
			visible := publicFields(edit, "id", "operation", "section", "reason", "requirementIds")
			// Keep the old UI's expected shape during rollout without sending source text.
			visible["before"] = ""
			visible["insertAfter"] = ""
			visible["sourceQuotes"] = []any{}
			switch edit["operation"] {
			case "replace", "add", "move", "merge":
				visible["after"] = edit["after"]
			case "remove":
				visible["after"] = ""
			default:
				// Retained passages reproduce the original resume.
				return nil
			}
			return visible
		})
	}
	if sections, ok := report["sections"].([]any); ok {
		public["sections"] = publicRows(sections, func(section map[string]any) map[string]any {
			visible := publicFields(section, "heading", "editIds")
			visible["retained"] = []any{}
			return visible
		})
	}
	if cleanup, ok := report["cleanup"].([]any); ok {
		public["cleanup"] = publicRows(cleanup, func(note map[string]any) map[string]any {
			return publicFields(note, "text", "severity")
		})
	}
	out["report"] = public
	return out
}

func publicFields(value map[string]any, fields ...string) map[string]any {
	out := make(map[string]any, len(fields))
	for _, field := range fields {
		if item, ok := value[field]; ok {
			out[field] = item
		}
	}
	return out
}

func publicRows(rows []any, project func(map[string]any) map[string]any) []any {
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		if item, ok := row.(map[string]any); ok {
			if visible := project(item); visible != nil {
				out = append(out, visible)
			}
		}
	}
	return out
}
