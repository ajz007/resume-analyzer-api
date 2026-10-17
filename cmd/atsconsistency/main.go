// Command atsconsistency checks that ATS Readiness does not depend on the job
// description. For each resume/JD pair it runs the analysis prompt without the
// JD (ATS check) and with it (job match), computes ATS Readiness the same way
// the API does, and fails if the two differ by more than -max-delta points.
//
// Layout of -pairs:
//
//	pairs/
//	  backend-go/
//	    resume.pdf   (or resume.docx)
//	    jd.txt
//	  data-analyst/
//	    ...
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"resume-backend/internal/analyses"
	"resume-backend/internal/extract"
	"resume-backend/internal/llm"
	openai "resume-backend/internal/llm/openai"
	"resume-backend/internal/shared/config"
)

func main() {
	cfg := config.Load()

	pairsDir := flag.String("pairs", "", "Directory of resume/JD pair subdirectories")
	promptVersion := flag.String("prompt-version", "v2_5", "Prompt version (v2_3, v2_4 or v2_5)")
	model := flag.String("model", cfg.LLMModel, "LLM model")
	runs := flag.Int("runs", 1, "Runs per mode for each pair")
	maxDelta := flag.Float64("max-delta", 3, "Maximum allowed ATS Readiness difference between modes")
	flag.Parse()

	if strings.TrimSpace(*pairsDir) == "" {
		exitErr("-pairs is required")
	}
	if *runs < 1 {
		exitErr("-runs must be at least 1")
	}

	client, err := openai.NewClient(os.Getenv("OPENAI_API_KEY"), *model)
	if err != nil {
		exitErr(err.Error())
	}

	entries, err := os.ReadDir(*pairsDir)
	if err != nil {
		exitErr(fmt.Sprintf("read pairs dir: %v", err))
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		exitErr("no pair subdirectories found")
	}

	ctx := context.Background()
	failed := 0
	fmt.Printf("%-24s %10s %10s %8s\n", "pair", "ats-only", "with-jd", "delta")
	for _, name := range names {
		dir := filepath.Join(*pairsDir, name)
		resumeText, jd, err := loadPair(ctx, dir)
		if err != nil {
			fmt.Printf("%-24s error: %v\n", name, err)
			failed++
			continue
		}

		atsOnly, err := averageScore(ctx, client, *promptVersion, resumeText, "", *runs)
		if err != nil {
			fmt.Printf("%-24s error (ats-only): %v\n", name, err)
			failed++
			continue
		}
		withJD, err := averageScore(ctx, client, *promptVersion, resumeText, jd, *runs)
		if err != nil {
			fmt.Printf("%-24s error (with-jd): %v\n", name, err)
			failed++
			continue
		}

		delta := withJD - atsOnly
		mark := ""
		if math.Abs(delta) > *maxDelta {
			mark = "  FAIL"
			failed++
		}
		fmt.Printf("%-24s %10.1f %10.1f %+8.1f%s\n", name, atsOnly, withJD, delta, mark)
	}

	if failed > 0 {
		exitErr(fmt.Sprintf("%d of %d pairs failed (max delta %.1f)", failed, len(names), *maxDelta))
	}
	fmt.Printf("all %d pairs within %.1f points\n", len(names), *maxDelta)
}

func loadPair(ctx context.Context, dir string) (resumeText, jd string, err error) {
	var resumePath, mimeType string
	for _, candidate := range []struct{ name, mime string }{
		{"resume.pdf", "application/pdf"},
		{"resume.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	} {
		if _, statErr := os.Stat(filepath.Join(dir, candidate.name)); statErr == nil {
			resumePath, mimeType = filepath.Join(dir, candidate.name), candidate.mime
			break
		}
	}
	if resumePath == "" {
		return "", "", fmt.Errorf("missing resume.pdf or resume.docx")
	}

	resumeBytes, err := os.ReadFile(resumePath)
	if err != nil {
		return "", "", fmt.Errorf("read resume: %w", err)
	}
	resumeText, err = extract.ExtractTextFromBytes(ctx, resumeBytes, mimeType, filepath.Base(resumePath))
	if err != nil {
		return "", "", fmt.Errorf("extract resume text: %w", err)
	}

	jdBytes, err := os.ReadFile(filepath.Join(dir, "jd.txt"))
	if err != nil {
		return "", "", fmt.Errorf("read jd.txt: %w", err)
	}
	if strings.TrimSpace(string(jdBytes)) == "" {
		return "", "", fmt.Errorf("jd.txt is empty")
	}
	return resumeText, string(jdBytes), nil
}

func averageScore(ctx context.Context, client llm.Client, promptVersion, resumeText, jd string, runs int) (float64, error) {
	total := 0.0
	for i := 0; i < runs; i++ {
		score, err := atsReadiness(ctx, client, promptVersion, resumeText, jd)
		if err != nil {
			return 0, err
		}
		total += score
	}
	return total / float64(runs), nil
}

func atsReadiness(ctx context.Context, client llm.Client, promptVersion, resumeText, jd string) (float64, error) {
	input := llm.AnalyzeInput{ResumeText: resumeText, JobDescription: jd, PromptVersion: promptVersion}

	var raw json.RawMessage
	var err error
	switch promptVersion {
	case "v2_3":
		raw, err = analyses.ValidateV2_3WithRetry(ctx, client, input)
	case "v2_4", "v2_5":
		raw, err = analyses.ValidateV2_4WithRetry(ctx, client, input)
	default:
		return 0, fmt.Errorf("unsupported prompt version: %s", promptVersion)
	}
	if err != nil {
		return 0, err
	}

	// v2_3, v2_4 and v2_5 share the same ats block.
	var parsed analyses.AnalysisResultV2_3
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return 0, fmt.Errorf("decode output: %w", err)
	}
	score, ok := analyses.CalculateATSReadinessScore(parsed.ATS.ScoreExplanation)
	if !ok {
		return 0, fmt.Errorf("atsReadability/resumeStructure components missing from output")
	}
	return score, nil
}

func exitErr(msg string) {
	_, _ = fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
