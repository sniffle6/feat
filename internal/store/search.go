package store

import (
	"fmt"
	"strings"
)

// SearchOpts controls filtering and output for search queries.
type SearchOpts struct {
	Scope     []string // entity types to include (nil = all)
	FeatureID string   // limit to one feature (empty = all)
	Verbose   bool     // full text vs snippets
	Limit     int      // max results (0 = default 20)
}

// SearchResult is a single match from the FTS5 search index.
type SearchResult struct {
	EntityType string  `json:"entity_type"`
	EntityID   string  `json:"entity_id"`
	FeatureID  string  `json:"feature_id"`
	FieldName  string  `json:"field_name"`
	Snippet    string  `json:"snippet"`
	Rank       float64 `json:"rank"`
}

// sanitizeQuery quotes queries containing path-like characters that break FTS5
// bareword parsing (e.g., auth.go, cmd/docket/main.go). Leaves FTS5 syntax
// (quotes, wildcards, boolean operators) untouched.
func sanitizeQuery(query string) string {
	if strings.ContainsAny(query, `"*`) ||
		strings.Contains(query, " AND ") || strings.Contains(query, " OR ") ||
		strings.Contains(query, " NOT ") || strings.Contains(query, " NEAR") {
		return query
	}
	if strings.ContainsAny(query, "./\\:") {
		return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
	}
	return query
}

// Search queries the FTS5 search index and returns ranked results.
func (s *Store) Search(query string, opts SearchOpts) ([]SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("search query cannot be empty")
	}
	query = sanitizeQuery(query)

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	// Build the content column expression: snippet or raw content
	contentExpr := "snippet(search_index, 4, '[', ']', '...', 12)"
	if opts.Verbose {
		contentExpr = "content"
	}

	// Base query with FTS5 MATCH
	q := fmt.Sprintf(
		`SELECT entity_type, entity_id, feature_id, field_name, %s, rank
		 FROM search_index
		 WHERE search_index MATCH ?`,
		contentExpr,
	)
	args := []interface{}{query}

	// Optional filters
	if len(opts.Scope) > 0 {
		placeholders := make([]string, len(opts.Scope))
		for i, sc := range opts.Scope {
			placeholders[i] = "?"
			args = append(args, sc)
		}
		q += " AND entity_type IN (" + strings.Join(placeholders, ",") + ")"
	}
	if opts.FeatureID != "" {
		q += " AND feature_id = ?"
		args = append(args, opts.FeatureID)
	}

	q += " ORDER BY rank LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.EntityType, &r.EntityID, &r.FeatureID, &r.FieldName, &r.Snippet, &r.Rank); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// RebuildSearchIndex drops and repopulates the FTS5 index from source tables.
func (s *Store) RebuildSearchIndex() error {
	if _, err := s.db.Exec("DELETE FROM search_index"); err != nil {
		return fmt.Errorf("clear search index: %w", err)
	}
	populateSearchIndex(s.db)
	return nil
}
