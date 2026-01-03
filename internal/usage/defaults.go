package usage

import "time"

func defaultUsage() Usage {
	return Usage{
		Plan:     "Starter",
		Limit:    10,
		Used:     0,
		ResetsAt: time.Now().UTC().Add(7 * 24 * time.Hour),
	}
}
