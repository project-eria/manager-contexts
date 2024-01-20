package lib

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
)

type LibTestSuite struct {
	suite.Suite
}

func Test_LibTestSuite(t *testing.T) {
	suite.Run(t, &LibTestSuite{})
}

func (ts *LibTestSuite) SetupTest() {
	zerolog.SetGlobalLevel(zerolog.Disabled)
}

func (ts *LibTestSuite) Test_NoDailyContexts() {
	now := time.Date(2024, time.January, 1, 1, 0, 0, 0, time.UTC) // 2024-01-01 - Monday
	context := []string{"holiday"}

	current, removed, added := GetDailyContexts(now, context)
	ts.Equal([]string{"holiday", "monday", "weekday"}, current)
	ts.Equal([]string{"monday", "weekday"}, added)
	ts.Equal([]string{}, removed)
}

func (ts *LibTestSuite) Test_NoNewContexts() {
	now := time.Date(2024, time.January, 1, 1, 0, 0, 0, time.UTC) // 2024-01-01 - Monday
	context := []string{"holiday", "monday", "weekday"}

	current, removed, added := GetDailyContexts(now, context)
	ts.Equal([]string{"holiday", "monday", "weekday"}, current)
	ts.Equal([]string{}, added)
	ts.Equal([]string{}, removed)
}

func (ts *LibTestSuite) Test_NewDayContexts() {
	now := time.Date(2024, time.January, 2, 1, 0, 0, 0, time.UTC) // 2024-01-01 - Tuesday
	context := []string{"holiday", "monday", "weekday"}

	current, removed, added := GetDailyContexts(now, context)
	ts.Equal([]string{"holiday", "tuesday", "weekday"}, current)
	ts.Equal([]string{"tuesday"}, added)
	ts.Equal([]string{"monday"}, removed)
}

func (ts *LibTestSuite) Test_NewWeekContexts() {
	now := time.Date(2024, time.January, 1, 1, 0, 0, 0, time.UTC) // 2024-01-01 - Monday
	context := []string{"holiday", "sunday", "weekend"}

	current, removed, added := GetDailyContexts(now, context)
	ts.Equal([]string{"holiday", "monday", "weekday"}, current)
	ts.Equal([]string{"monday", "weekday"}, added)
	ts.Equal([]string{"sunday", "weekend"}, removed)
}
