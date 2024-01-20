package lib

import (
	"strings"
	"time"

	"github.com/gookit/goutil/arrutil"
)

func GetDailyContexts(now time.Time, currentContexts []string) ([]string, []string, []string) {
	day := strings.ToLower(now.Weekday().String())
	// Add the day
	dailyContexts := []string{day}
	if day == "saturday" || day == "sunday" {
		dailyContexts = append(dailyContexts, "weekend")
	} else {
		dailyContexts = append(dailyContexts, "weekday")
	}

	// Remove the daily contexts, to keep only the other contexts
	otherContexts := arrutil.Excepts(currentContexts, []string{"weekday", "weekend", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}, arrutil.StringEqualsComparer)

	// Merge the new contexts
	current := arrutil.Union(otherContexts, dailyContexts, arrutil.StringEqualsComparer)
	removed := arrutil.Excepts(currentContexts, current, arrutil.StringEqualsComparer)
	added := arrutil.Excepts(current, currentContexts, arrutil.StringEqualsComparer)
	return current, removed, added
}
