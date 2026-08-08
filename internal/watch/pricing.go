package watch

import "time"

// Rates are US dollars per million tokens, from Anthropic's published pricing.
// Cache rates are not published separately: a read costs 0.1x the model's input
// rate, a 5-minute write 1.25x, and a 1-hour write 2x. Deriving them keeps one
// number per model to maintain instead of five.
const (
	perMillion       = 1_000_000
	cacheReadFactor  = 0.1
	cacheWrite5mCost = 1.25
	cacheWrite1hCost = 2.0
)

type rate struct{ input, output float64 }

var modelRates = map[string]rate{
	"claude-opus-5":     {5, 25},
	"claude-opus-4-8":   {5, 25},
	"claude-opus-4-7":   {5, 25},
	"claude-opus-4-6":   {5, 25},
	"claude-fable-5":    {10, 50},
	"claude-mythos-5":   {10, 50},
	"claude-sonnet-5":   {3, 15},
	"claude-sonnet-4-6": {3, 15},
	"claude-haiku-4-5":  {1, 5},
}

// Fast mode is a separate product at a separate price. Only Opus 5's fast rate
// is published, so a fast turn on any other model prices as unknown rather than
// silently billing at the standard rate.
var fastRates = map[string]rate{
	"claude-opus-5": {10, 50},
}

// Sonnet 5 carries introductory pricing through 2026-08-31; turns before the
// cutoff bill at the lower rate, turns after it at list.
var (
	sonnet5IntroRate = rate{2, 10}
	sonnet5IntroEnds = time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
)

// rateFor resolves the price of one turn. ok is false when the model is unknown
// to this table — a new model must show as unpriced, never as free.
func rateFor(model, speed string, ts int64) (rate, bool) {
	if model == "" {
		return rate{}, false
	}
	if speed == "fast" {
		r, ok := fastRates[model]
		return r, ok
	}
	if model == "claude-sonnet-5" && ts > 0 && time.UnixMilli(ts).Before(sonnet5IntroEnds) {
		return sonnet5IntroRate, true
	}
	r, ok := modelRates[model]
	return r, ok
}

// cost prices one turn's usage. The caller has already resolved the rate, so a
// turn whose model is unknown never reaches here.
func (r rate) cost(u usage) float64 {
	write5m, write1h := u.cacheWrites()
	dollars := float64(u.InputTokens) * r.input
	dollars += float64(u.OutputTokens) * r.output
	dollars += float64(u.CacheReadInputTokens) * r.input * cacheReadFactor
	dollars += float64(write5m) * r.input * cacheWrite5mCost
	dollars += float64(write1h) * r.input * cacheWrite1hCost
	return dollars / perMillion
}

// cacheWrites splits cache creation by TTL. The nested breakdown is the
// authority when present; older lines carry only the total, which is charged at
// the 5-minute rate — the cheaper of the two, so an unknown TTL under-reports
// rather than inventing spend.
func (u usage) cacheWrites() (fiveMinute, oneHour int) {
	if u.CacheCreation.Ephemeral5m > 0 || u.CacheCreation.Ephemeral1h > 0 {
		return u.CacheCreation.Ephemeral5m, u.CacheCreation.Ephemeral1h
	}
	return u.CacheCreationInputTokens, 0
}
