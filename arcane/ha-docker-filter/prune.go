package main

import (
	"encoding/json"
	"net/http"
)

// excludeFromPrune adds label!=<key> to a prune's filters, so that it spares
// Home Assistant's objects. Hiding does nothing for a prune: dockerd decides
// what is unused, and a stopped app has no container, so its image is unused
// by definition. It reports false when it has answered the request itself.
func excludeFromPrune(w http.ResponseWriter, r *http.Request, key string) bool {
	// dockerd reads a form body too and prefers it to the query string, so
	// decide on both and send neither: only the query built here goes on.
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	query := r.URL.Query()
	for name, values := range r.PostForm {
		query[name] = values
	}
	filters, err := parseFilters(query.Get("filters"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid filter: "+err.Error())
		return false
	}
	if len(filters["label!"]) > 0 {
		// dockerd reads several label! values as "has all of them", so
		// adding one here would spare less than either does alone.
		writeError(w, http.StatusBadRequest,
			"ha-docker-filter: this prune has a label! filter of its own, which dockerd cannot combine with the one that spares Home Assistant ("+key+")")
		return false
	}
	filters["label!"] = map[string]bool{key: true}
	encoded, _ := json.Marshal(filters)
	query.Set("filters", string(encoded))
	r.URL.RawQuery = query.Encode()
	dropBody(r)
	return true
}

// parseFilters reads dockerd's filters parameter in both of its forms:
// {"label":{"a=b":true}} and the older {"label":["a=b"]}.
func parseFilters(s string) (map[string]map[string]bool, error) {
	filters := map[string]map[string]bool{}
	if s == "" {
		return filters, nil
	}
	if err := json.Unmarshal([]byte(s), &filters); err == nil {
		if filters == nil {
			filters = map[string]map[string]bool{}
		}
		return filters, nil
	}
	var lists map[string][]string
	if err := json.Unmarshal([]byte(s), &lists); err != nil {
		return nil, err
	}
	filters = map[string]map[string]bool{}
	for name, values := range lists {
		filters[name] = map[string]bool{}
		for _, v := range values {
			filters[name][v] = true
		}
	}
	return filters, nil
}
