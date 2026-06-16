package modelv1

const SchemaVersion = "resume.v1"

// ResumeModel is the canonical structured resume content model for v1.
type ResumeModel struct {
	SchemaVersion  string          `json:"schemaVersion"`
	Basics         Basics          `json:"basics"`
	Target         Target          `json:"target"`
	Summary        Summary         `json:"summary"`
	Skills         []SkillCategory `json:"skills"`
	Experience     []Experience    `json:"experience"`
	Projects       []Project       `json:"projects"`
	Education      []Education     `json:"education"`
	Certifications []Certification `json:"certifications"`
	Achievements   []Achievement   `json:"achievements"`
	CustomSections []CustomSection `json:"customSections"`
	SectionOrder   []string        `json:"sectionOrder"`
}

type SectionKey string

const (
	SectionSummary        SectionKey = "summary"
	SectionSkills         SectionKey = "skills"
	SectionExperience     SectionKey = "experience"
	SectionProjects       SectionKey = "projects"
	SectionEducation      SectionKey = "education"
	SectionCertifications SectionKey = "certifications"
	SectionAchievements   SectionKey = "achievements"
	SectionCustomSections SectionKey = "customSections"
)

type Basics struct {
	FullName string   `json:"fullName"`
	Headline string   `json:"headline"`
	Email    string   `json:"email"`
	Phone    string   `json:"phone"`
	Location Location `json:"location"`
	Links    []Link   `json:"links"`
}

type Location struct {
	City    string `json:"city"`
	State   string `json:"state"`
	Country string `json:"country"`
}

type Link struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

type Target struct {
	RoleTitle string `json:"roleTitle"`
	Seniority string `json:"seniority"`
	Persona   string `json:"persona"`
	Industry  string `json:"industry"`
}

type Summary struct {
	Text string `json:"text"`
}

type SkillCategory struct {
	Category string      `json:"category"`
	Items    []SkillItem `json:"items"`
}

type SkillItem struct {
	Name   string `json:"name"`
	Level  string `json:"level"`
	Years  *int   `json:"years"`
	Source string `json:"source"`
}

type Experience struct {
	ID             string      `json:"id"`
	Company        string      `json:"company"`
	Title          string      `json:"title"`
	Location       string      `json:"location"`
	EmploymentType string      `json:"employmentType"`
	StartDate      string      `json:"startDate"`
	EndDate        string      `json:"endDate"`
	IsCurrent      bool        `json:"isCurrent"`
	Summary        string      `json:"summary"`
	Highlights     []Highlight `json:"highlights"`
	Technologies   []string    `json:"technologies"`
}

type Highlight struct {
	ID     string   `json:"id"`
	Text   string   `json:"text"`
	Tags   []string `json:"tags"`
	Source string   `json:"source"`
}

type Project struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Description  string      `json:"description"`
	Role         string      `json:"role"`
	Highlights   []Highlight `json:"highlights"`
	Technologies []string    `json:"technologies"`
	Links        []Link      `json:"links"`
}

type Education struct {
	ID           string   `json:"id"`
	Institution  string   `json:"institution"`
	Degree       string   `json:"degree"`
	FieldOfStudy string   `json:"fieldOfStudy"`
	StartDate    string   `json:"startDate"`
	EndDate      string   `json:"endDate"`
	Location     Location `json:"location"`
}

type Certification struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Issuer        string `json:"issuer"`
	IssueDate     string `json:"issueDate"`
	ExpiryDate    string `json:"expiryDate"`
	CredentialURL string `json:"credentialUrl"`
}

type Achievement struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type CustomSection struct {
	ID    string              `json:"id"`
	Title string              `json:"title"`
	Items []CustomSectionItem `json:"items"`
}

type CustomSectionItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}
