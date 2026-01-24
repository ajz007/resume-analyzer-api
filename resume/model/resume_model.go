package model

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ResumeModel represents the canonical resume payload.
type ResumeModel struct {
	Header         ResumeHeader          `json:"header"`
	Summary        []string              `json:"summary"`
	Skills         ResumeSkills          `json:"skills"`
	Experience     []ResumeExperience    `json:"experience"`
	Projects       []ResumeProject       `json:"projects"`
	Education      []ResumeEducation     `json:"education"`
	Achievements   []ResumeAchievement   `json:"achievements"`
	Certifications []ResumeCertification `json:"certifications"`
}

// Validate enforces required fields and formatting rules for ResumeModel.
func (m ResumeModel) Validate() error {
	if strings.TrimSpace(m.Header.Name) == "" {
		return errors.New("fullName is required")
	}
	if strings.TrimSpace(m.Header.Nationality) != "" || strings.TrimSpace(m.Header.MaritalStatus) != "" {
		return errors.New("sensitive fields like nationality or maritalStatus are not allowed")
	}
	for i, link := range m.Header.Links {
		if !isFullURL(strings.TrimSpace(link)) {
			return fmt.Errorf("links[%d] must be a full URL", i)
		}
	}
	for i, exp := range m.Experience {
		if err := validateDateField(exp.Start, fmt.Sprintf("experience[%d].start", i)); err != nil {
			return err
		}
		if err := validateDateField(exp.End, fmt.Sprintf("experience[%d].end", i)); err != nil {
			return err
		}
	}
	for i, project := range m.Projects {
		if err := validateDateField(project.Start, fmt.Sprintf("projects[%d].start", i)); err != nil {
			return err
		}
		if err := validateDateField(project.End, fmt.Sprintf("projects[%d].end", i)); err != nil {
			return err
		}
	}
	for i, edu := range m.Education {
		if err := validateDateField(edu.Start, fmt.Sprintf("education[%d].start", i)); err != nil {
			return err
		}
		if err := validateDateField(edu.End, fmt.Sprintf("education[%d].end", i)); err != nil {
			return err
		}
	}
	for i, achievement := range m.Achievements {
		if err := validateDateField(achievement.Date, fmt.Sprintf("achievements[%d].date", i)); err != nil {
			return err
		}
	}
	for i, cert := range m.Certifications {
		if err := validateDateField(cert.Date, fmt.Sprintf("certifications[%d].date", i)); err != nil {
			return err
		}
		if err := validateDateField(cert.Expires, fmt.Sprintf("certifications[%d].expires", i)); err != nil {
			return err
		}
	}
	return nil
}

// ResumeHeader captures top-of-resume contact and identity details.
type ResumeHeader struct {
	Name          string   `json:"name"`
	Title         string   `json:"title"`
	Email         string   `json:"email"`
	Phone         string   `json:"phone"`
	Location      string   `json:"location"`
	Links         []string `json:"links"`
	Nationality   string   `json:"nationality,omitempty"`
	MaritalStatus string   `json:"maritalStatus,omitempty"`
}

// ResumeSkills groups skills by category.
type ResumeSkills struct {
	Languages     []string `json:"languages"`
	Frameworks    []string `json:"frameworks"`
	Databases     []string `json:"databases"`
	CloudDevOps   []string `json:"cloudDevOps"`
	Observability []string `json:"observability"`
	Tools         []string `json:"tools"`
}

// ResumeExperience represents a work history entry.
type ResumeExperience struct {
	ID         string   `json:"id"`
	Company    string   `json:"company"`
	Role       string   `json:"role"`
	Location   string   `json:"location"`
	Start      string   `json:"start"`
	End        string   `json:"end"`
	Highlights []string `json:"highlights"`
}

// ResumeProject represents a notable project.
type ResumeProject struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Highlights  []string `json:"highlights"`
}

// ResumeEducation represents an education entry.
type ResumeEducation struct {
	Institution string   `json:"institution"`
	Degree      string   `json:"degree"`
	Field       string   `json:"field"`
	Location    string   `json:"location"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Highlights  []string `json:"highlights"`
}

// ResumeAchievement represents a discrete achievement.
type ResumeAchievement struct {
	Title      string   `json:"title"`
	Date       string   `json:"date"`
	Highlights []string `json:"highlights"`
}

// ResumeCertification represents a certification entry.
type ResumeCertification struct {
	Name    string `json:"name"`
	Issuer  string `json:"issuer"`
	Date    string `json:"date"`
	Expires string `json:"expires"`
}

var resumeDatePattern = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

func isFullURL(value string) bool {
	if value == "" {
		return false
	}
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(value)), "TO-FILL:") {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

func validateDateField(value, field string) error {
	if value == "" || value == "Present" {
		return nil
	}
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(value)), "TO-FILL:") {
		return nil
	}
	if !resumeDatePattern.MatchString(value) {
		return fmt.Errorf("%s must be YYYY-MM or Present", field)
	}
	return nil
}
