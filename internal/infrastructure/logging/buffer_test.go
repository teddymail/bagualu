package logging

import "testing"

func TestBufferBoundsAndSince(t *testing.T) {
	buffer := NewBuffer(2)
	writer := buffer.Writer("mihomo", "stderr")
	_, _ = writer.Write([]byte("first\nsecond\nthird"))
	entries := buffer.Entries(Query{Service: "mihomo", Limit: 10})
	if len(entries) != 2 || entries[0].Message != "second" || entries[1].Message != "first" {
		t.Fatalf("unexpected bounded entries: %#v", entries)
	}
	_, _ = writer.Write([]byte("\n"))
	entries = buffer.Entries(Query{Since: entries[0].ID, Limit: 10})
	if len(entries) != 1 || entries[0].Message != "third" {
		t.Fatalf("unexpected incremental entries: %#v", entries)
	}
}
