// coveragecheck consolidates Go coverage blocks and enforces a statement threshold.
// Cross-package instrumentation can emit the same block from multiple test binaries.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type block struct{ statements, count int64 }
type profile struct {
	mode   string
	blocks map[string]block
}

func readProfile(r io.Reader) (profile, error) {
	result := profile{blocks: map[string]block{}}
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return result, fmt.Errorf("empty coverage profile")
	}
	result.mode = strings.TrimPrefix(scanner.Text(), "mode: ")
	if result.mode != "set" && result.mode != "count" && result.mode != "atomic" {
		return result, fmt.Errorf("invalid coverage mode")
	}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return result, fmt.Errorf("invalid coverage line: %s", scanner.Text())
		}
		statements, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || statements < 0 {
			return result, fmt.Errorf("invalid statement count")
		}
		count, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || count < 0 {
			return result, fmt.Errorf("invalid execution count")
		}
		previous, exists := result.blocks[fields[0]]
		if exists && previous.statements != statements {
			return result, fmt.Errorf("conflicting coverage blocks: %s", fields[0])
		}
		// The gate measures whether a block ran. Keep the largest observed count
		// so repeated blocks do not inflate either the denominator or hit counts.
		result.blocks[fields[0]] = block{statements, max(previous.count, count)}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	if len(result.blocks) == 0 {
		return result, fmt.Errorf("coverage profile contains no blocks")
	}
	return result, nil
}
func (p profile) totals() (covered, total int64) {
	for _, b := range p.blocks {
		total += b.statements
		if b.count > 0 {
			covered += b.statements
		}
	}
	return
}
func (p profile) write(w io.Writer) error {
	if _, err := fmt.Fprintln(w, "mode: "+p.mode); err != nil {
		return err
	}
	keys := make([]string, 0, len(p.blocks))
	for key := range p.blocks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b := p.blocks[key]
		if _, err := fmt.Fprintf(w, "%s %d %d\n", key, b.statements, b.count); err != nil {
			return err
		}
	}
	return nil
}
func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("coveragecheck", flag.ContinueOnError)
	path := flags.String("profile", "coverage.out", "coverage profile to normalize and check")
	minimum := flags.Float64("min", 80, "minimum statement coverage percentage")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *minimum < 0 || *minimum > 100 {
		return fmt.Errorf("min must be between 0 and 100")
	}
	file, err := os.Open(*path)
	if err != nil {
		return err
	}
	parsed, err := readProfile(file)
	_ = file.Close()
	if err != nil {
		return err
	}
	covered, total := parsed.totals()
	if total == 0 {
		return fmt.Errorf("profile contains no statements")
	}
	// Parse and validate before touching the original profile.
	target, err := os.Create(*path)
	if err != nil {
		return err
	}
	err = parsed.write(target)
	closeErr := target.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	percent := 100 * float64(covered) / float64(total)
	fmt.Fprintf(out, "Backend coverage: %.2f%% (%d/%d statements), required %.2f%%\n", percent, covered, total, *minimum)
	if percent < *minimum {
		return fmt.Errorf("backend coverage is below the required threshold")
	}
	return nil
}
func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
