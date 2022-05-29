package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"strings"
	"testing"
)

func BenchmarkNthFromEnd(b *testing.B) {
	big := make([]byte, 1<<20)
	for i := 0; i < len(big); i++ {
		big[i] = byte(i & 0xff)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		NthFromEnd(big, 'a', 1<<10)
	}
}

func tmpFilename(base string) string {
	fn := fmt.Sprintf("%s%x", base, rand.Int63())
	return path.Join(os.TempDir(), fn)
}

func TestLastNth(t *testing.T) {
	var Trials = []struct {
		hay      string
		needle   byte
		count    int
		expected int
	}{
		{` 1   ~ 2   ~ 3   ~ 4   ~!5   ~ 6   ~ 7   ~ 8   ~ 9   ~ 10  ~ 11  ~ 12  ~ 13  ~`, '~', 10, 23},
		{"~~~~~~~~~~~", '~', 10, 1},
		{"~ ~ ~ ~ ~ ~ ~ ~ ~ ~", '~', 10, 0},
		{"~ ~ ~ ~ ~ ~ ~ ~ ~", '~', 10, -1},
		{"~", '~', 1000, -1},
		{"", '~', 100, -1},
		{"1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n", '\n', 10, 9},
	}

	for _, tr := range Trials {
		actual := NthFromEnd([]byte(tr.hay), tr.needle, tr.count)
		if actual != tr.expected {
			t.Errorf("expected %d, actual %d", tr.expected, actual)
		}
	}
}

func TestTrimIdempotency(t *testing.T) {
	var lineCount = 1000
	var extra = lineCount * 2
	var buf bytes.Buffer

	for i := 0; i < lineCount+extra; i++ {
		fmt.Fprintf(&buf, "%05d\n", i)
	}

	fn := tmpFilename("TestTrimIdempotency-")
	defer os.Remove(fn)
	if err := os.WriteFile(fn, buf.Bytes(), 0600); err != nil {
		t.Error("Couldn't write test data")
	}

	// try it a few times
	for i := 0; i < 2; i++ {
		if err := trimFile(fn, lineCount); err != nil {
			t.Error("Couldn't trim test file")
		}
	}

	content, err := ioutil.ReadFile(fn)
	if err != nil {
		t.Error("Couldn't read test data back from file", err)
	}

	// NB strings.Split returns an empty string when the last character is
	// the split target; ie, strings.Split("a~b~", "~") = ["a" "b" ""]
	// The tests below adjust for that
	lines := strings.Split(string(content), "\n")
	targets := strings.Split(buf.String(), "\n")

	if len(lines) != lineCount+1 {
		t.Errorf("File contains wrong number of lines (%d, expected %d)", len(lines), lineCount)
	}

	if lines[0] != targets[extra] || lines[lineCount-1] != targets[lineCount+extra-1] {
		t.Error("Wrong first or last line",
			lines[0], targets[extra],
			lines[lineCount-1], targets[lineCount+extra-1])
	}
}

func TestTrimHistory(t *testing.T) {
	var lineCount = 100
	var extra = lineCount * 2
	var buf bytes.Buffer

	for i := 0; i < lineCount+extra; i++ {
		fmt.Fprintf(&buf, "%05d\n", i)
	}

	fn := tmpFilename("testTrimHistory-")
	defer os.Remove(fn)
	if err := os.WriteFile(fn, buf.Bytes(), 0600); err != nil {
		t.Error("Couldn't write test data")
	}

	trimFile(fn, lineCount)
	content, err := ioutil.ReadFile(fn)
	if err != nil {
		t.Error("Couldn't read test data back from file", err)
	}

	// NB strings.Split returns an empty string when the last character is
	// the split target; eg, strings.Split("a~b~", "~") = ["a", "b", ""]
	// The tests below adjust for that
	lines := strings.Split(string(content), "\n")
	targets := strings.Split(buf.String(), "\n")

	if len(lines) != lineCount+1 {
		t.Errorf("File contains wrong number of lines (%d, expected %d", len(lines), lineCount)
	}

	if lines[0] != targets[extra] || lines[lineCount-1] != targets[lineCount+extra-1] {
		t.Error("Wrong first or last line",
			lines[0], targets[extra],
			lines[lineCount-1], targets[lineCount+extra-1])
	}
}
