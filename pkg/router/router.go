// Package router implements the core semantic routing logic.
// It matches incoming requests to the appropriate backend based on
// semantic similarity of the request content.
package router

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Route represents a named route with an associated set of example phrases
// used to determine semantic similarity.
type Route struct {
	// Name is the unique identifier for this route.
	Name string

	// Utterances are example phrases that represent this route.
	// Incoming requests are matched against these phrases.
	Utterances []string

	// Threshold is the minimum similarity score required to match this route.
	// Values should be between 0.0 and 1.0. Defaults to 0.8 if not set.
	Threshold float64
}

// Match represents the result of a routing decision.
type Match struct {
	// Route is the matched route, or nil if no route matched.
	Route *Route

	// Score is the similarity score of the best match (0.0 to 1.0).
	Score float64
}

// Encoder defines the interface for generating embeddings from text.
type Encoder interface {
	// Encode generates an embedding vector for the given text.
	Encode(ctx context.Context, text string) ([]float32, error)
}

// Router routes incoming queries to the best matching Route using
// semantic similarity via embeddings.
type Router struct {
	mu       sync.RWMutex
	routes   []*Route
	encoder  Encoder
	// routeEmbeddings maps route name -> averaged embedding of its utterances.
	routeEmbeddings map[string][]float32
}

// New creates a new Router with the given encoder.
func New(encoder Encoder) *Router {
	return &Router{
		encoder:         encoder,
		routeEmbeddings: make(map[string][]float32),
	}
}

// AddRoute registers a route and pre-computes embeddings for its utterances.
func (r *Router) AddRoute(ctx context.Context, route *Route) error {
	if route.Name == "" {
		return errors.New("route name must not be empty")
	}
	if len(route.Utterances) == 0 {
		return fmt.Errorf("route %q must have at least one utterance", route.Name)
	}
	if route.Threshold == 0 {
		route.Threshold = 0.8
	}

	// Compute and average embeddings for all utterances.
	var avg []float32
	for _, utterance := range route.Utterances {
		emb, err := r.encoder.Encode(ctx, utterance)
		if err != nil {
			return fmt.Errorf("encoding utterance for route %q: %w", route.Name, err)
		}
		if avg == nil {
			avg = make([]float32, len(emb))
		}
		for i, v := range emb {
			avg[i] += v
		}
	}
	n := float32(len(route.Utterances))
	for i := range avg {
		avg[i] /= n
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = append(r.routes, route)
	r.routeEmbeddings[route.Name] = avg
	return nil
}

// Route finds the best matching route for the given query.
// Returns a Match with a nil Route if no route meets its threshold.
func (r *Router) Route(ctx context.Context, query string) (*Match, error) {
	queryEmb, err := r.encoder.Encode(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("encoding query: %w", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	best := &Match{}
	for _, route := range r.routes {
		emb := r.routeEmbeddings[route.Name]
		score := cosineSimilarity(queryEmb, emb)
		if score > best.Score {
			best.Score = score
			best.Route = route
		}
	}

	// Return nil route if best score doesn't meet the route's threshold.
	if best.Route != nil && best.Score < best.Route.Threshold {
		return &Match{Score: best.Score}, nil
	}
	return best, nil
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (sqrt32(normA) * sqrt32(normB))
}

// sqrt32 computes the square root of a float32.
func sqrt32(x float32) float32 {
	if x <= 0 {
		return 0
	}
	// Newton-Raphson approximation.
	z := x
	for i := 0; i < 10; i++ {
		z -= (z*z - x) / (2 * z)
	}
	return z
}
