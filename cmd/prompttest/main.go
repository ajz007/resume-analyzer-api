package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"resume-backend/internal/analyses"
	"resume-backend/internal/extract"
	"resume-backend/internal/llm"
	openai "resume-backend/internal/llm/openai"
	"resume-backend/internal/shared/config"
)

func main() {
	cfg := config.Load()

	resumePath := flag.String("resume", "", "Path to resume file (pdf or docx)")
	jdPath := flag.String("jd", "", "Path to job description file (optional)")
	promptVersion := flag.String("prompt-version", "v1", "Prompt version")
	outPath := flag.String("out", "", "Path to write raw JSON output (optional)")
	provider := flag.String("provider", cfg.LLMProvider, "LLM provider")
	model := flag.String("model", cfg.LLMModel, "LLM model")
	flag.Parse()

	if strings.TrimSpace(*resumePath) == "" {
		exitErr("resume path is required")
	}

	mimeType, err := mimeFromExt(*resumePath)
	if err != nil {
		exitErr(err.Error())
	}

	resumeBytes, err := os.ReadFile(*resumePath)
	if err != nil {
		exitErr(fmt.Sprintf("read resume: %v", err))
	}
	fileName := filepath.Base(*resumePath)

	resumeText, err := extract.ExtractTextFromBytes(context.Background(), resumeBytes, mimeType, fileName)
	if err != nil {
		exitErr(fmt.Sprintf("extract resume text: %v", err))
	}

	jobDescription := ""
	if strings.TrimSpace(*jdPath) != "" {
		jdBytes, err := os.ReadFile(*jdPath)
		if err != nil {
			exitErr(fmt.Sprintf("read job description: %v", err))
		}
		jobDescription = string(jdBytes)
	}

	client, err := buildClient(*provider, *model)
	if err != nil {
		exitErr(err.Error())
	}

	input := llm.AnalyzeInput{
		ResumeText:     resumeText,
		JobDescription: jobDescription,
		PromptVersion:  *promptVersion,
		TargetRole:     "",
	}

	var raw json.RawMessage

	switch strings.TrimSpace(*promptVersion) {
	case "v1":
		raw, err = client.AnalyzeResume(context.Background(), input)
		if err != nil {
			exitErr(fmt.Sprintf("llm analyze: %v", err))
		}
		var parsed analyses.AnalysisResultV1
		if err := json.Unmarshal(raw, &parsed); err != nil {
			exitErr(fmt.Sprintf("invalid json: %v", err))
		}
	case "v2":
		raw, err = analyses.ValidateV2WithRetry(context.Background(), client, input)
		if err != nil {
			exitErr(fmt.Sprintf("v2 schema: %v", err))
		}
	case "v2_1":
		raw, err = client.AnalyzeResume(context.Background(), input)
		if err != nil {
			exitErr(fmt.Sprintf("llm analyze: %v", err))
		}
		var parsed analyses.AnalysisResultV2_1
		if err := json.Unmarshal(raw, &parsed); err != nil {
			exitErr(fmt.Sprintf("invalid json: %v", err))
		}
		if err := parsed.Validate(); err != nil {
			exitErr(fmt.Sprintf("invalid v2_1 schema: %v", err))
		}
	case "v2_2":
		raw, err = analyses.ValidateV2_2WithRetry(context.Background(), client, input)
		if err != nil {
			exitErr(fmt.Sprintf("v2_2 schema: %v", err))
		}
	case "v2_3":
		raw, err = analyses.ValidateV2_3WithRetry(context.Background(), client, input)
		if err != nil {
			exitErr(fmt.Sprintf("v2_3 schema: %v", err))
		}
	default:
		exitErr(fmt.Sprintf("unsupported prompt version: %s", *promptVersion))
	}

	pretty, err := prettyJSON(raw)
	if err != nil {
		exitErr(fmt.Sprintf("format json: %v", err))
	}

	if *outPath != "" {
		if err := os.WriteFile(*outPath, pretty, 0o644); err != nil {
			exitErr(fmt.Sprintf("write output: %v", err))
		}
	}

	if _, err := os.Stdout.Write(pretty); err != nil {
		exitErr(fmt.Sprintf("write stdout: %v", err))
	}
	if len(pretty) == 0 || pretty[len(pretty)-1] != '\n' {
		_, _ = os.Stdout.Write([]byte("\n"))
	}
}

func buildClient(provider, model string) (llm.Client, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "openai":
		return openai.NewClient(os.Getenv("OPENAI_API_KEY"), model)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func mimeFromExt(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return "application/pdf", nil
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", nil
	default:
		return "", fmt.Errorf("unsupported resume file type: %s", filepath.Ext(path))
	}
}

func prettyJSON(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func exitErr(msg string) {
	_, _ = fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
