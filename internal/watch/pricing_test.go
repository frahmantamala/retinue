package watch

import (
	"math"
	"testing"
	"time"
)

func ms(s string) int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t.UnixMilli()
}

func TestRateFor(t *testing.T) {
	cases := []struct {
		name         string
		model, speed string
		ts           int64
		want         rate
		wantOK       bool
	}{
		{"opus 5", "claude-opus-5", "standard", 0, rate{5, 25}, true},
		{"opus 4.8", "claude-opus-4-8", "standard", 0, rate{5, 25}, true},
		{"haiku", "claude-haiku-4-5", "", 0, rate{1, 5}, true},
		{"fable", "claude-fable-5", "", 0, rate{10, 50}, true},
		{"opus 5 fast is its own product", "claude-opus-5", "fast", 0, rate{10, 50}, true},
		{"unknown model", "claude-something-9", "standard", 0, rate{}, false},
		{"empty model", "", "standard", 0, rate{}, false},
		// Only Opus 5's fast rate is published; anything else must not be
		// priced at the standard rate just because the model is known.
		{"fast on a model with no published fast rate", "claude-sonnet-5", "fast", 0, rate{}, false},
		{"sonnet 5 during the intro window", "claude-sonnet-5", "", ms("2026-08-09T00:00:00Z"), rate{2, 10}, true},
		{"sonnet 5 after the intro window", "claude-sonnet-5", "", ms("2026-09-02T00:00:00Z"), rate{3, 15}, true},
		{"sonnet 5 with no timestamp falls back to list", "claude-sonnet-5", "", 0, rate{3, 15}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := rateFor(c.model, c.speed, c.ts)
			if ok != c.wantOK || got != c.want {
				t.Errorf("want %+v/%v, got %+v/%v", c.want, c.wantOK, got, ok)
			}
		})
	}
}

func TestCostPricesEachBucketSeparately(t *testing.T) {
	var u usage
	u.InputTokens = 1000
	u.OutputTokens = 500
	u.CacheReadInputTokens = 10000
	u.CacheCreation.Ephemeral5m = 2000
	u.CacheCreation.Ephemeral1h = 3000

	// opus rates 5/25 per Mtok:
	//   input        1000 * 5          =  5,000
	//   output        500 * 25         = 12,500
	//   cache read  10000 * 5 * 0.1    =  5,000
	//   write 5m     2000 * 5 * 1.25   = 12,500
	//   write 1h     3000 * 5 * 2.0    = 30,000
	//                                    ------
	//                                    65,000 / 1e6 = $0.065
	got := rate{5, 25}.cost(u)
	if math.Abs(got-0.065) > 1e-9 {
		t.Errorf("want 0.065, got %v", got)
	}
}

// A 1-hour cache write costs 2x input and a 5-minute write 1.25x. Real sessions
// use 1h, so collapsing the two understates the bill by a wide margin.
func TestCacheTTLsPriceDifferently(t *testing.T) {
	var short, long usage
	short.CacheCreation.Ephemeral5m = 100_000
	long.CacheCreation.Ephemeral1h = 100_000

	r := rate{5, 25}
	if s, l := r.cost(short), r.cost(long); math.Abs(l-s*1.6) > 1e-9 {
		t.Errorf("1h should cost 1.6x a 5m write of the same size: 5m=%v 1h=%v", s, l)
	}
}

// Older lines carry only the cache-creation total with no TTL breakdown.
func TestCacheWriteFallsBackToTheCheaperTTL(t *testing.T) {
	var u usage
	u.CacheCreationInputTokens = 2000

	five, hour := u.cacheWrites()
	if five != 2000 || hour != 0 {
		t.Fatalf("want the total charged at the 5m rate, got 5m=%d 1h=%d", five, hour)
	}
	if got := (rate{5, 25}).cost(u); math.Abs(got-0.0125) > 1e-9 {
		t.Errorf("want 0.0125, got %v", got)
	}
}

// The nested breakdown wins when present — otherwise a session reporting both
// the total and the split would be billed twice.
func TestCacheWritePrefersTheBreakdown(t *testing.T) {
	var u usage
	u.CacheCreationInputTokens = 5000
	u.CacheCreation.Ephemeral1h = 5000

	if five, hour := u.cacheWrites(); five != 0 || hour != 5000 {
		t.Errorf("want the split to win, got 5m=%d 1h=%d", five, hour)
	}
}

// A model this build has no rate for must surface as unpriced, never as free.
func TestUnknownModelCountsAsUnpriced(t *testing.T) {
	g := NewGraph("", "")
	g.EnsureLead("lead")
	g.Apply(LeadNodeID, mustDecode(t, `{"type":"assistant","uuid":"a","message":{"model":"claude-from-the-future","usage":{"input_tokens":100,"output_tokens":200}}}`))

	n := nodesOf(g)[LeadNodeID]
	if n.Cost != 0 {
		t.Errorf("an unknown model must not contribute cost: %v", n.Cost)
	}
	if n.Unpriced != 300 {
		t.Errorf("want 300 unpriced tokens, got %d", n.Unpriced)
	}
	if n.Model != "claude-from-the-future" {
		t.Errorf("model should still be reported: %q", n.Model)
	}
}

func TestKnownModelAccumulatesCost(t *testing.T) {
	g := NewGraph("", "")
	g.EnsureLead("lead")
	line := `{"type":"assistant","uuid":"a","message":{"model":"claude-opus-5","usage":{"input_tokens":1000,"output_tokens":500}}}`
	g.Apply(LeadNodeID, mustDecode(t, line))
	g.Apply(LeadNodeID, mustDecode(t, line))

	n := nodesOf(g)[LeadNodeID]
	if math.Abs(n.Cost-0.035) > 1e-9 { // (5,000 + 12,500) * 2 / 1e6
		t.Errorf("want 0.035 across two turns, got %v", n.Cost)
	}
	if n.Unpriced != 0 {
		t.Errorf("priced turns must not count as unpriced: %d", n.Unpriced)
	}
}
