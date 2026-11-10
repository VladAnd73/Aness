package budget

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestWeeklyUsedReadsTheEndpoint(t *testing.T) {
	u, _ := dailyFixture(t)
	u.setWeekly(42.5, time.Now().Add(48*time.Hour))
	got, err := WeeklyUsed(context.Background())
	if err != nil || got != 42.5 {
		t.Errorf("WeeklyUsed = %v, %v; want 42.5", got, err)
	}
}

func TestWeeklyUsedFailsClosed(t *testing.T) {
	u, _ := dailyFixture(t)
	u.set(http.StatusTooManyRequests, "slow down")
	if _, err := WeeklyUsed(context.Background()); err == nil {
		t.Errorf("WeeklyUsed on HTTP 429 returned no error")
	}
	u.set(http.StatusOK, `{"five_hour":{"utilization":3}}`)
	if _, err := WeeklyUsed(context.Background()); err == nil {
		t.Errorf("WeeklyUsed without seven_day returned no error")
	}
	t.Setenv("AGENT_BUDGET_CREDS", "/nonexistent/creds.json")
	if _, err := WeeklyUsed(context.Background()); err == nil {
		t.Errorf("WeeklyUsed without credentials returned no error")
	}
}

// TestLiveWeeklyUsed makes one real usage-endpoint call and logs its
// latency:
//
//	SPORE_BUDGET_LIVE=1 go test ./internal/budget/ -run TestLiveWeeklyUsed -v
func TestLiveWeeklyUsed(t *testing.T) {
	if os.Getenv("SPORE_BUDGET_LIVE") != "1" {
		t.Skip("set SPORE_BUDGET_LIVE=1 to read the real usage endpoint once")
	}
	start := time.Now()
	used, err := WeeklyUsed(context.Background())
	t.Logf("weekly used %.1f%%, err %v, latency %v", used, err, time.Since(start))
	if err != nil {
		t.Fatal(err)
	}
}
