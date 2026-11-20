package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// maxList bounds a list answer held in memory. A larger one is refused: a
// list is never passed on unfiltered.
const maxList = 256 << 20

// readBody takes the whole answer. An error here becomes a 502.
func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
		return nil, fmt.Errorf("answer is %s-encoded and cannot be filtered", enc)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxList+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxList {
		return nil, errors.New("answer is too large to filter")
	}
	return body, nil
}

func setBody(resp *http.Response, body []byte) {
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.TransferEncoding = nil
	resp.Header.Del("Transfer-Encoding")
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
}

// filterArray returns the JSON array without the elements hide reports, and
// how many it removed. Kept elements stay byte for byte as dockerd wrote
// them: decoding to Go values and back would round 64-bit sizes and re-escape
// text. With nothing removed the input comes back unchanged.
func filterArray(raw []byte, hide func(json.RawMessage) (bool, error)) ([]byte, int, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, 0, err
	}
	kept := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		hidden, err := hide(item)
		if err != nil {
			return nil, 0, err
		}
		if !hidden {
			kept = append(kept, item)
		}
	}
	removed := len(items) - len(kept)
	if removed == 0 {
		return raw, 0, nil
	}
	var out bytes.Buffer
	out.WriteByte('[')
	for i, item := range kept {
		if i > 0 {
			out.WriteByte(',')
		}
		out.Write(item)
	}
	out.WriteByte(']')
	out.Write(raw[len(bytes.TrimRight(raw, " \t\r\n")):]) // dockerd ends a body with a newline
	return out.Bytes(), removed, nil
}

func hideContainerElement(raw json.RawMessage) (bool, error) {
	var c struct{ Labels map[string]string }
	if err := json.Unmarshal(raw, &c); err != nil {
		return false, err
	}
	return hiddenContainer(c.Labels), nil
}

func hideImageElement(raw json.RawMessage, onlyHiddenUse map[string]bool) (bool, error) {
	var im struct {
		Id     string
		Labels map[string]string
	}
	if err := json.Unmarshal(raw, &im); err != nil {
		return false, err
	}
	return hiddenImage(im.Labels) || onlyHiddenUse[im.Id], nil
}

// containerList filters GET /containers/json.
func containerList(resp *http.Response) error {
	body, err := readBody(resp)
	if err != nil {
		return err
	}
	out, _, err := filterArray(body, hideContainerElement)
	if err != nil {
		return fmt.Errorf("container list: %w", err)
	}
	setBody(resp, out)
	return nil
}

// imageList filters GET /images/json.
func (f *filter) imageList(resp *http.Response) error {
	body, err := readBody(resp)
	if err != nil {
		return err
	}
	onlyHiddenUse, err := f.images.get(resp.Request.Context(), f.docker)
	if err != nil {
		return err
	}
	out, _, err := filterArray(body, func(raw json.RawMessage) (bool, error) {
		return hideImageElement(raw, onlyHiddenUse)
	})
	if err != nil {
		return fmt.Errorf("image list: %w", err)
	}
	setBody(resp, out)
	return nil
}

// imageCache remembers for a moment which images are run by hidden
// containers and by no other. Such an image may have no label to go by (an
// app that runs somebody else's image), and listed without its containers it
// would look unused.
type imageCache struct {
	mu   sync.Mutex
	at   time.Time
	only map[string]bool
}

func (c *imageCache) get(ctx context.Context, docker *http.Client) (map[string]bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.only != nil && time.Since(c.at) < 2*time.Second {
		return c.only, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/json?all=1", nil)
	if err != nil {
		return nil, err
	}
	resp, err := docker.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list containers: dockerd answered %d", resp.StatusCode)
	}
	var containers []struct {
		ImageID string
		Labels  map[string]string
	}
	if err := json.Unmarshal(body, &containers); err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	only := map[string]bool{}
	for _, ct := range containers {
		if hiddenContainer(ct.Labels) {
			only[ct.ImageID] = true
		}
	}
	for _, ct := range containers {
		if !hiddenContainer(ct.Labels) {
			delete(only, ct.ImageID)
		}
	}
	c.only, c.at = only, time.Now()
	return only, nil
}

// member is one key of a JSON object, with its value as written.
type member struct {
	key   string
	value json.RawMessage
}

// decodeObject returns an object's members in their order.
func decodeObject(raw []byte) ([]member, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil {
		return nil, err
	} else if tok != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}
	var out []member
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, errors.New("object key is not a string")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		out = append(out, member{key, value})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return out, nil
}

func encodeObject(members []member) []byte {
	var out bytes.Buffer
	out.WriteByte('{')
	for i, m := range members {
		if i > 0 {
			out.WriteByte(',')
		}
		key, _ := json.Marshal(m.key)
		out.Write(key)
		out.WriteByte(':')
		out.Write(m.value)
	}
	out.WriteByte('}')
	return out.Bytes()
}

// diskUsage filters GET /system/df. dockerd sends two shapes, and below API
// 1.52 or without verbose=1 both at once: the lists Containers and Images,
// and ContainerUsage and ImageUsage with counters over their Items. A tool
// may plan a prune from this answer and remove by ID, so a hidden entry left
// in it is an entry that can be pruned.
func (f *filter) diskUsage(resp *http.Response) error {
	body, err := readBody(resp)
	if err != nil {
		return err
	}
	top, err := decodeObject(body)
	if err != nil {
		return fmt.Errorf("disk usage: %w", err)
	}
	onlyHiddenUse, err := f.images.get(resp.Request.Context(), f.docker)
	if err != nil {
		return err
	}
	hideImage := func(raw json.RawMessage) (bool, error) { return hideImageElement(raw, onlyHiddenUse) }

	changed := false
	for i := range top {
		var out []byte
		var removed int
		switch top[i].key {
		case "Containers":
			out, removed, err = filterNullable(top[i].value, hideContainerElement)
		case "Images":
			out, removed, err = filterNullable(top[i].value, hideImage)
		case "ContainerUsage":
			out, removed, err = filterUsage(top[i].value, hideContainerElement, containerTotals)
		case "ImageUsage":
			out, removed, err = filterUsage(top[i].value, hideImage, imageTotals)
		default:
			continue
		}
		if err != nil {
			return fmt.Errorf("disk usage, %s: %w", top[i].key, err)
		}
		if removed > 0 {
			top[i].value = out
			changed = true
		}
	}
	if !changed {
		setBody(resp, body)
		return nil
	}
	setBody(resp, append(encodeObject(top), '\n'))
	return nil
}

func isNull(raw []byte) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

func filterNullable(raw []byte, hide func(json.RawMessage) (bool, error)) ([]byte, int, error) {
	if isNull(raw) {
		return raw, 0, nil
	}
	return filterArray(raw, hide)
}

// filterUsage removes hidden entries from a usage object's Items and counts
// again what dockerd counted from them (daemon/disk_usage.go at 29.8.2).
func filterUsage(raw []byte, hide func(json.RawMessage) (bool, error), totals func([]json.RawMessage) (map[string]int64, error)) ([]byte, int, error) {
	if isNull(raw) {
		return raw, 0, nil
	}
	obj, err := decodeObject(raw)
	if err != nil {
		return nil, 0, err
	}
	for i := range obj {
		if obj[i].key != "Items" || isNull(obj[i].value) {
			continue
		}
		items, removed, err := filterArray(obj[i].value, hide)
		if err != nil || removed == 0 {
			return raw, 0, err
		}
		obj[i].value = items
		var kept []json.RawMessage
		if err := json.Unmarshal(items, &kept); err != nil {
			return nil, 0, err
		}
		counted, err := totals(kept)
		if err != nil {
			return nil, 0, err
		}
		for j := range obj {
			if n, ok := counted[obj[j].key]; ok {
				obj[j].value = strconv.AppendInt(nil, n, 10)
			}
		}
		return encodeObject(obj), removed, nil
	}
	return raw, 0, nil // no Items: the counters are all there is, and they stay
}

func containerTotals(items []json.RawMessage) (map[string]int64, error) {
	t := map[string]int64{"TotalCount": int64(len(items)), "ActiveCount": 0, "TotalSize": 0, "Reclaimable": 0}
	for _, raw := range items {
		var c struct {
			State  string
			SizeRw int64
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		t["TotalSize"] += c.SizeRw
		switch c.State {
		case "running", "paused", "restarting":
			t["ActiveCount"]++
		default:
			t["Reclaimable"] += c.SizeRw
		}
	}
	return t, nil
}

// imageTotals leaves TotalSize alone: images share layers, so it is not the
// sum of anything in the list.
func imageTotals(items []json.RawMessage) (map[string]int64, error) {
	t := map[string]int64{"TotalCount": int64(len(items)), "ActiveCount": 0, "Reclaimable": 0}
	for _, raw := range items {
		var im struct{ Containers, Size, SharedSize int64 }
		if err := json.Unmarshal(raw, &im); err != nil {
			return nil, err
		}
		switch {
		case im.Containers > 0:
			t["ActiveCount"]++
		case im.Containers == 0 && im.Size != -1 && im.SharedSize != -1:
			t["Reclaimable"] += im.Size - im.SharedSize
		}
	}
	return t, nil
}
