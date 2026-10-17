package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeSharedBlocksWithoutInflatingCoverage(t *testing.T) {
	p, err := readProfile(strings.NewReader("mode: atomic\na.go:1.1,2.1 4 0\na.go:1.1,2.1 4 2\nb.go:1.1,2.1 1 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	covered, total := p.totals()
	if covered != 4 || total != 5 {
		t.Fatalf("got %d/%d", covered, total)
	}
	var out bytes.Buffer
	if err := p.write(&out); err != nil {
		t.Fatal(err)
	}
	again, err := readProfile(&out)
	if err != nil {
		t.Fatal(err)
	}
	a, b := again.totals()
	if a != covered || b != total {
		t.Fatal("normalization changed coverage")
	}
}
func TestRejectIncompleteOrMalformedProfiles(t *testing.T) {
	for _, input := range []string{"", "mode: set\n", "mode: invalid\na 1 0\n", "mode: set\na x 0\n", "mode: set\na 1 -1\n", "mode: set\na 1\n", "mode: set\na 1 0\na 2 1\n"} {
		if _, err := readProfile(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}
func TestGateChecksExactThresholdAndFailsBelow(t *testing.T) {
	for _, threshold := range []string{"80", "80.01"} {
		path := filepath.Join(t.TempDir(), "coverage.out")
		if err := os.WriteFile(path, []byte("mode: set\na.go:1.1,2.1 4 1\nb.go:1.1,2.1 1 0\n"), 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err := run([]string{"-profile", path, "-min", threshold}, &out)
		if (err != nil) != (threshold == "80.01") {
			t.Fatalf("threshold %s, error %v", threshold, err)
		}
	}
}
