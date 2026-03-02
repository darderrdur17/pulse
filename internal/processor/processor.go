package processor

import (
	"strings"
	"unicode"

	"github.com/derr/pulse/internal/models"
)

// stopWords are common English words excluded from keyword analysis
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "is": true, "it": true, "this": true, "that": true, "are": true,
	"was": true, "be": true, "by": true, "from": true, "as": true, "i": true,
	"we": true, "you": true, "he": true, "she": true, "they": true, "not": true,
	"have": true, "has": true, "had": true, "do": true, "does": true, "did": true,
	"will": true, "would": true, "could": true, "should": true, "may": true, "can": true,
}

// Processor handles normalisation and enrichment of raw posts
type Processor struct{}

// New returns a new Processor
func New() *Processor { return &Processor{} }

// EnrichPost adds keyword tags to a post in-place
func (p *Processor) EnrichPost(post *models.RawPost) {
	text := post.Title + " " + post.Body
	post.Tags = ExtractTopKeywords(text, 10)
}

// EnrichBatch runs EnrichPost over a slice of posts
func (p *Processor) EnrichBatch(posts []models.RawPost) []models.RawPost {
	enriched := make([]models.RawPost, len(posts))
	for i, post := range posts {
		p.EnrichPost(&post)
		enriched[i] = post
	}
	return enriched
}

// ExtractTopKeywords tokenises text and returns the most frequent meaningful tokens
func ExtractTopKeywords(text string, topN int) []string {
	freq := make(map[string]int)
	words := tokenise(text)
	for _, w := range words {
		if !stopWords[w] && len(w) > 2 {
			freq[w]++
		}
	}

	// Simple insertion sort to find top N (avoids importing sort for small N)
	type kv struct {
		key   string
		count int
	}
	ranked := make([]kv, 0, len(freq))
	for k, v := range freq {
		ranked = append(ranked, kv{k, v})
	}
	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && ranked[j].count > ranked[j-1].count; j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}

	keywords := make([]string, 0, topN)
	for i := 0; i < len(ranked) && i < topN; i++ {
		keywords = append(keywords, ranked[i].key)
	}
	return keywords
}

// tokenise lowercases text and splits on non-alphanumeric characters
func tokenise(text string) []string {
	lower := strings.ToLower(text)
	return strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// CalcEngagementScore returns a single weighted engagement figure
func CalcEngagementScore(score, numComments int) float64 {
	// Comments weighted 2× — they indicate deeper engagement than upvotes
	return float64(score) + float64(numComments)*2.0
}
