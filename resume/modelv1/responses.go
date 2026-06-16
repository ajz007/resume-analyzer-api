package modelv1

// ResumeGenerationResponse is the strict contract future AI resume generation
// calls must return. AI output must not use arbitrary keys for missing data;
// use RequiresUserInput with a stable field path instead.
//
// Anti-fabrication rules for future generation logic:
// AI must not invent companies, degrees, or certifications.
// AI must not add unsupported skills silently.
// AI should ask for user input or record assumptions when source data is incomplete.
type ResumeGenerationResponse struct {
	Resume            ResumeModel         `json:"resume"`
	RequiresUserInput []RequiresUserInput `json:"requiresUserInput"`
	Assumptions       []Assumption        `json:"assumptions"`
	Warnings          []ResponseWarning   `json:"warnings"`
}

type RequiresUserInput struct {
	Field    string `json:"field"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type Assumption struct {
	Message string `json:"message"`
}

type ResponseWarning struct {
	Message string `json:"message"`
}

// ResumeTailoringResponse is the strict contract future AI resume tailoring
// calls must return. Unsupported job-description requirements belong in
// MissingRequirements instead of being silently fabricated into the resume.
//
// Anti-fabrication rules for future tailoring logic:
// AI must not invent companies, degrees, or certifications.
// AI must not add unsupported skills silently.
// AI must put unsupported JD requirements into MissingRequirements.
type ResumeTailoringResponse struct {
	TailoredResume      ResumeModel          `json:"tailoredResume"`
	Changes             []TailoringChange    `json:"changes"`
	MissingRequirements []MissingRequirement `json:"missingRequirements"`
	Warnings            []ResponseWarning    `json:"warnings"`
}

type TailoringChange struct {
	Section    string `json:"section"`
	ItemID     string `json:"itemId"`
	ChangeType string `json:"changeType"`
	Before     string `json:"before"`
	After      string `json:"after"`
	Reason     string `json:"reason"`
	Risk       string `json:"risk"`
}

type MissingRequirement struct {
	Requirement    string `json:"requirement"`
	Recommendation string `json:"recommendation"`
}
