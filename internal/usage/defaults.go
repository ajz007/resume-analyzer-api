package usage

import (
	"strings"
	"time"
)

const (
	GuestMonthlyAnalysisLimit    = 3
	FreeUserMonthlyAnalysisLimit = 15

	PlanGuest   = "Guest"
	PlanStarter = "Starter"
)

func defaultUsage(userID string) Usage {
	now := time.Now().UTC()
	return Usage{
		Plan:     defaultPlan(userID),
		Limit:    defaultLimit(userID),
		Used:     0,
		ResetsAt: nextMonthlyReset(now),
	}
}

func defaultPlan(userID string) string {
	if IsGuestUserID(userID) {
		return PlanGuest
	}
	return PlanStarter
}

func defaultLimit(userID string) int {
	if IsGuestUserID(userID) {
		return GuestMonthlyAnalysisLimit
	}
	return FreeUserMonthlyAnalysisLimit
}

func IsGuestUserID(userID string) bool {
	return strings.HasPrefix(strings.TrimSpace(userID), "guest:")
}

func nextMonthlyReset(now time.Time) time.Time {
	now = now.UTC()
	year, month, _ := now.Date()
	return time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)
}
