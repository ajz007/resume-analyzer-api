package llm

import _ "embed"

var (
	//go:embed prompts/v1.txt
	promptV1 string
	//go:embed prompts/v2.txt
	promptV2 string
	//go:embed prompts/v2_1.txt
	promptV2_1 string
	//go:embed prompts/v2_2.txt
	promptV2_2 string
	//go:embed prompts/v2_3.txt
	promptV2_3 string
)

// PromptTemplate returns the prompt template text and whether the version was recognized.
func PromptTemplate(version string) (string, bool) {
	switch version {
	case "v2_3":
		return promptV2_3, true
	case "v2_2":
		return promptV2_2, true
	case "v2_1":
		return promptV2_1, true
	case "v2":
		return promptV2, true
	case "v1":
		return promptV1, true
	default:
		return promptV1, false
	}
}
