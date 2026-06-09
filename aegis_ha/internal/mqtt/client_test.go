package mqtt

import (
	"bufio"
	"bytes"
	"testing"
)

func TestRemainingLengthRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 127, 128, 16383, 16384, 2097151} {
		enc := appendRemainingLength(nil, n)
		br := bufio.NewReader(bytes.NewReader(enc))
		got, err := readRemainingLength(br)
		if err != nil {
			t.Fatalf("n=%d decode: %v", n, err)
		}
		if got != n {
			t.Errorf("n=%d round-trip got %d", n, got)
		}
	}
}

func TestAppendStringPrefix(t *testing.T) {
	b := appendString(nil, "MQTT")
	if len(b) != 6 || b[0] != 0 || b[1] != 4 || string(b[2:]) != "MQTT" {
		t.Fatalf("bad string encoding: %v", b)
	}
}

func TestTopicMatch(t *testing.T) {
	cases := []struct {
		filter, topic string
		want          bool
	}{
		{"aegis_ha/panel/cmd", "aegis_ha/panel/cmd", true},
		{"aegis_ha/panel/cmd", "aegis_ha/panel/state", false},
		{"aegis_ha/+/set", "aegis_ha/exit_delay/set", true},
		{"aegis_ha/+/set", "aegis_ha/panel/cmd", false},
		{"homeassistant/status", "homeassistant/status", true},
		{"aegis_ha/#", "aegis_ha/a/b/c", true},
		{"aegis_ha/#", "other/a", false},
		{"aegis_ha/+/set", "aegis_ha/a/b/set", false},
	}
	for _, c := range cases {
		if got := topicMatch(c.filter, c.topic); got != c.want {
			t.Errorf("match(%q,%q)=%v want %v", c.filter, c.topic, got, c.want)
		}
	}
}

func TestPublishFailsWhenDisconnected(t *testing.T) {
	c := New(Options{Broker: "127.0.0.1:1", ClientID: "x"})
	if err := c.Publish("t", []byte("p"), false); err == nil {
		t.Fatal("publish should fail when not connected")
	}
}
