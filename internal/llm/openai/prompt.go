package openai

import (
	"fmt"
	"log"
	"strings"

	"resume-backend/internal/llm"
)

// Message represents an OpenAI chat message.
type Message struct {
	Role    string
	Content string
}

const (
	systemPromptStrict  = "You are a resume analysis engine. Respond with JSON only. Output must match the schema exactly."
	systemPromptV2      = "You are a resume analysis engine. Respond with JSON only. No markdown. Never omit keys. Output must match the schema exactly."
	systemPromptFixJSON = "You are a JSON repair tool. Return only valid JSON that matches the schema exactly."
)

// BuildPrompt creates the chat messages for a resume analysis request.
func BuildPrompt(promptVersion string, resumeText string, jobDescription string, model string) []Message {
	usedVersion, developer := resolvePromptTemplate(promptVersion, jobDescription, model)
	system := systemPromptStrict
	if usedVersion == "v2" {
		system = systemPromptV2
	}

	return []Message{
		{Role: "system", Content: system},
		{Role: "developer", Content: developer},
		{Role: "user", Content: buildUserPrompt(resumeText, jobDescription)},
	}
}

func buildFixPrompt(promptVersion string, jobDescription string, model string, raw []byte) []Message {
	_, developer := resolvePromptTemplate(promptVersion, jobDescription, model)
	return []Message{
		{Role: "system", Content: systemPromptFixJSON},
		{Role: "developer", Content: developer},
		{Role: "user", Content: fixUserPrompt(raw)},
	}
}

func resolvePromptTemplate(promptVersion string, jobDescription string, model string) (string, string) {
	version := strings.TrimSpace(promptVersion)
	template, ok := llm.PromptTemplate(version)
	usedVersion := version
	if !ok {
		log.Printf("unknown prompt version %q, defaulting to v1", version)
		usedVersion = "v1"
		template, _ = llm.PromptTemplate(usedVersion)
	}

	jobDescriptionProvided := "true"
	if strings.TrimSpace(jobDescription) == "" {
		jobDescriptionProvided = "false"
	}

	replacer := strings.NewReplacer(
		"{{PROMPT_VERSION}}", usedVersion,
		"{{MODEL}}", model,
		"{{JOB_DESCRIPTION_PROVIDED}}", jobDescriptionProvided,
	)
	return usedVersion, replacer.Replace(template)
}

func buildUserPrompt(resumeText, jobDescription string) string {
	jd := jobDescription
	if strings.TrimSpace(jd) == "" {
		jd = "N/A"
	}
	return fmt.Sprintf("Resume Text:\n%s\n\nJob Description:\n%s", resumeText, jd)
}

func fixUserPrompt(raw []byte) string {
	return fmt.Sprintf("Fix this JSON to match the schema exactly. Output JSON only:\n%s", string(raw))
}

func prependSystemMessage(messages []Message, content string) []Message {
	if strings.TrimSpace(content) == "" {
		return messages
	}
	out := make([]Message, 0, len(messages)+1)
	out = append(out, Message{Role: "system", Content: content})
	out = append(out, messages...)
	return out
}
