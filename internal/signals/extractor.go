package signals

import (
	"math"
	"strings"
	"time"

	"github.com/derr/pulse/internal/models"
)

// defaultTopicKeywords are used when no custom keywords are provided
var defaultTopicKeywords = map[string][]string{
	"tech_interest":    {"golang", "python", "rust", "kubernetes", "docker", "api", "database", "backend", "frontend", "cloud"},
	"finance_interest": {"stock", "crypto", "bitcoin", "investment", "trading", "market", "fund", "etf", "portfolio", "yield"},
	"ai_interest":      {"llm", "gpt", "ai", "machine", "learning", "neural", "model", "inference", "training", "transformer"},
	"high_influence":   {"show", "ask", "launch", "release", "announce", "new", "open", "source", "build", "create"},
}

// Extractor computes intelligence signals from user profiles and posts
type Extractor struct {
	keywords map[string][]string
}

// New returns a new Extractor with built-in topic keywords
func New() *Extractor { return NewWithKeywords(nil) }

// NewWithKeywords returns an Extractor that uses the given topic keywords for scoring.
// If keywords is nil or empty, built-in defaults are used. Use config.LoadTopicKeywords("config/topic_keywords.json") to load from file.
func NewWithKeywords(keywords map[string][]string) *Extractor {
	if len(keywords) == 0 {
		keywords = defaultTopicKeywords
	}
	return &Extractor{keywords: keywords}
}

// ComputeProfileSignals scores a single UserProfile against all signal dimensions
func (e *Extractor) ComputeProfileSignals(p *models.UserProfile) {
	// 1. Topic affinity signals — derived from keyword overlap
	for signalName, keywords := range e.keywords {
		p.Signals[signalName] = keywordOverlapScore(p.TopKeywords, keywords)
	}

	// 2. Influence signal — normalised engagement relative to post count
	if p.PostCount > 0 {
		rawInfluence := p.AvgScore / 1000.0 // normalise against ~1k engagement ceiling
		p.Signals["influence_score"] = clamp(rawInfluence, 0, 1)
	}

	// 3. Activity consistency — how spread out are active hours? (entropy-based)
	p.Signals["activity_consistency"] = hourEntropy(p.ActiveHours)

	// 4. Recency signal — how recently was the user active?
	daysSinceActive := time.Since(p.LastSeen).Hours() / 24
	p.Signals["recency"] = clamp(1.0-daysSinceActive/30.0, 0, 1) // decay over 30 days
}

// ComputePostSignals returns a list of signals for a single post
func (e *Extractor) ComputePostSignals(post *models.RawPost) []models.Signal {
	var sigs []models.Signal
	now := time.Now().UTC()

	// Virality signal
	engagementScore := float64(post.Score+post.NumComments*2) / 5000.0
	sigs = append(sigs, models.Signal{
		Name:        "virality",
		Score:       clamp(engagementScore, 0, 1),
		Confidence:  0.8,
		Description: "Engagement-weighted virality estimate",
		ComputedAt:  now,
	})

	// Freshness signal
	ageHours := time.Since(post.CreatedAt).Hours()
	freshness := math.Exp(-ageHours / 24.0) // exponential decay; half-life ≈ 24h
	sigs = append(sigs, models.Signal{
		Name:        "freshness",
		Score:       clamp(freshness, 0, 1),
		Confidence:  0.95,
		Description: "Exponential decay freshness (24h half-life)",
		ComputedAt:  now,
	})

	return sigs
}

// keywordOverlapScore measures the Jaccard-style overlap between two keyword sets
func keywordOverlapScore(userKW, targetKW []string) float64 {
	if len(userKW) == 0 || len(targetKW) == 0 {
		return 0
	}
	target := make(map[string]bool, len(targetKW))
	for _, k := range targetKW {
		target[strings.ToLower(k)] = true
	}
	matches := 0
	for _, k := range userKW {
		if target[strings.ToLower(k)] {
			matches++
		}
	}
	return clamp(float64(matches)/float64(len(targetKW)), 0, 1)
}

// hourEntropy computes Shannon entropy over hourly activity distribution (normalised 0–1)
func hourEntropy(hours map[int]int) float64 {
	if len(hours) == 0 {
		return 0
	}
	total := 0
	for _, v := range hours {
		total += v
	}
	entropy := 0.0
	for _, v := range hours {
		if v == 0 {
			continue
		}
		p := float64(v) / float64(total)
		entropy -= p * math.Log2(p)
	}
	// Max entropy for 24 bins = log2(24) ≈ 4.58
	return clamp(entropy/math.Log2(24), 0, 1)
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
