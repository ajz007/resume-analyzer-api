package llm

import _ "embed"

var (
	//go:embed prompts/resume_gen_v1.txt
	resumeGenPromptV1 string
)

// ResumeGenPromptV1 returns the prompt used to generate ResumeModel JSON.
func ResumeGenPromptV1() string {
	return resumeGenPromptV1
}
