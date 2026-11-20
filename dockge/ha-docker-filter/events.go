package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// eventStream filters GET /events: records about hidden containers are
// dropped, every other record goes on as dockerd wrote it. dockerd writes one
// record per line in each of its four framings; application/json-seq puts
// the byte 0x1E in front.
func eventStream(resp *http.Response) error {
	resp.Body = &eventFilter{src: resp.Body, lines: bufio.NewReaderSize(resp.Body, 64<<10)}
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	return nil
}

type eventFilter struct {
	src     io.ReadCloser
	lines   *bufio.Reader
	pending []byte
}

func (e *eventFilter) Read(p []byte) (int, error) {
	for len(e.pending) == 0 {
		line, err := e.lines.ReadBytes('\n')
		if len(line) > 0 && !hiddenEvent(line) {
			e.pending = line
		}
		if err != nil {
			if len(e.pending) == 0 {
				return 0, err
			}
			break // hand over the last record; the error comes on the next call
		}
	}
	n := copy(p, e.pending)
	e.pending = e.pending[n:]
	return n, nil
}

func (e *eventFilter) Close() error { return e.src.Close() }

// hiddenEvent reports whether a record is a container event of a hidden
// container. A container event carries the container's labels as attributes,
// so no lookup is needed. Anything it cannot read passes.
func hiddenEvent(line []byte) bool {
	var ev struct {
		Type  string
		Actor struct{ Attributes map[string]string }
	}
	record := bytes.TrimLeft(line, "\x1e \t\r\n")
	if json.Unmarshal(record, &ev) != nil {
		return false
	}
	return ev.Type == "container" && hiddenContainer(ev.Actor.Attributes)
}
