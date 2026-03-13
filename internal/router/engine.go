package router

import (
	"context"
	"errors"
	"log"
	"sort"
	"strconv"
	"sync"
	"unicode/utf8"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/models"
)

const maxLogErrorLen = 120

// truncateForLog shortens s for readable one-line logs; preserves runes.
func truncateForLog(s string, max int) string {
	if max <= 0 {
		max = maxLogErrorLen
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i] + " …"
		}
		n++
	}
	return s + " …"
}

// ErrNoRoutes is returned when no adapter returns a valid route for the request.
var ErrNoRoutes = errors.New("no available routes for the requested pair")

// Quote returns routes from all adapters in parallel, scored by fees and estimated time (best first).
// Preferences.Priority can be "cheapest" (default) or "fastest".
// Preferences.AllowedBridges, if set, restricts which adapters are queried.
func Quote(ctx context.Context, adapters []bridges.Adapter, req models.QuoteRequest) ([]models.Route, error) {
	allowed := make(map[string]bool)
	if req.Preferences != nil && len(req.Preferences.AllowedBridges) > 0 {
		for _, id := range req.Preferences.AllowedBridges {
			allowed[id] = true
		}
	}

	var filtered []bridges.Adapter
	for _, a := range adapters {
		if len(allowed) == 0 || allowed[a.ID()] {
			filtered = append(filtered, a)
		}
	}
	adapters = filtered

	if len(adapters) == 0 {
		return nil, ErrNoRoutes
	}

	var mu sync.Mutex
	var routes []*models.Route
	var wg sync.WaitGroup

	for _, a := range adapters {
		adapter := a
		wg.Add(1)
		go func() {
			defer wg.Done()
			route, err := adapter.GetQuote(ctx, req)
			if err != nil {
				log.Printf("[router] quote adapter=%s err=%s", adapter.ID(), truncateForLog(err.Error(), maxLogErrorLen))
				return
			}
			if route == nil || len(route.Hops) == 0 {
				return
			}
			mu.Lock()
			routes = append(routes, route)
			mu.Unlock()
		}()
	}

	wg.Wait()

	if len(routes) == 0 {
		return nil, ErrNoRoutes
	}

	priority := "cheapest"
	if req.Preferences != nil && req.Preferences.Priority != "" {
		priority = req.Preferences.Priority
	}

	// Score and sort: lower fee and lower time are better.
	for _, r := range routes {
		r.Score = scoreRoute(r, priority)
	}
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Score > routes[j].Score
	})

	out := make([]models.Route, len(routes))
	for i, r := range routes {
		out[i] = *r
	}
	return out, nil
}

func scoreRoute(r *models.Route, priority string) float64 {
	fee, _ := strconv.ParseFloat(r.TotalFee, 64)
	timeNorm := float64(r.EstimatedTimeSeconds) / 60.0
	if timeNorm < 1 {
		timeNorm = 1
	}
	switch priority {
	case "fastest":
		return 1000.0 / timeNorm
	case "cheapest", "":
		fallthrough
	default:
		return 1000.0 / (1 + fee)
	}
}
