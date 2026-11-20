package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// excludeFromPrune adds label!=<key> to a prune's filters, so that it spares
// Home Assistant's objects. Hiding does nothing for a prune: dockerd decides
// what is unused, and a stopped app has no container, so its image is unused
// by definition. list is the endpoint that lists what this prune removes. It
// reports false when it has answered the request itself.
func (f *filter) excludeFromPrune(w http.ResponseWriter, r *http.Request, list, key string) bool {
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
	if own := filters["label!"]; len(own) > 0 {
		// dockerd reads several label! values as "has all of them", so the
		// tool's and this one together would spare less than either alone.
		// The tool's can go when nothing carries them: they spare nothing
		// then. Otherwise both cannot be kept, and nothing is pruned.
		n, err := f.carrying(r.Context(), list, own)
		if err != nil {
			log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
			writeError(w, http.StatusBadGateway, "ha-docker-filter: "+err.Error())
			return false
		}
		if n > 0 {
			names := make([]string, 0, len(own))
			for name := range own {
				names = append(names, name)
			}
			sort.Strings(names)
			writeError(w, http.StatusConflict, fmt.Sprintf(
				"ha-docker-filter: nothing was pruned. This prune keeps what is labelled %s, and %d of the %s carry that. Docker cannot keep those and Home Assistant's in one prune.",
				strings.Join(names, " and "), n, strings.TrimSuffix(strings.TrimPrefix(list, "/"), "/json")))
			return false
		}
	}
	filters["label!"] = map[string]bool{key: true}
	encoded, _ := json.Marshal(filters)
	query.Set("filters", string(encoded))
	r.URL.RawQuery = query.Encode()
	dropBody(r)
	return true
}

// carrying counts what a list endpoint returns for label=<labels>. dockerd
// matches a list's label and a prune's label! the same way, so this is what
// a prune with label!=<labels> would spare.
func (f *filter) carrying(ctx context.Context, list string, labels map[string]bool) (int, error) {
	encoded, _ := json.Marshal(map[string]map[string]bool{"label": labels})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://docker"+list+"?all=1&filters="+url.QueryEscape(string(encoded)), nil)
	if err != nil {
		return 0, err
	}
	resp, err := f.docker.Do(req)
	if err != nil {
		return 0, err
	}
	body, err := readBody(resp)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%s answered %d", list, resp.StatusCode)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		return 0, fmt.Errorf("%s: %w", list, err)
	}
	return len(items), nil
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
