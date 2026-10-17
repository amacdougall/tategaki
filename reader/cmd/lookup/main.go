// Command lookup exercises the lookup engine from a terminal.
//
//	lookup --scan "蜘蛛の巣が張って"        scan from the start, as a tap would
//	lookup --scan "…" --offset 3          scan from code point 3
//	lookup -n 5 食べ                       exact selection (span adjust) lookup
//
// Options go before the selection (Go flag parsing stops at the first word).
//
// It finds data/jmdict.sqlite relative to the working directory or the repo,
// or takes --db.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amacdougall/tategaki/reader/internal/dict"
)

func findDB() string {
	for _, p := range []string{"data/jmdict.sqlite", "../data/jmdict.sqlite", "/mnt/us/tategaki/jmdict.sqlite"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "jmdict.sqlite")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "data/jmdict.sqlite"
}

func main() {
	scanText := flag.String("scan", "", "text to scan (as if tapped at --offset)")
	offset := flag.Int("offset", 0, "tap offset into --scan text, in code points")
	policy := flag.String("policy", dict.PolicyLongest, "ranking policy: longest | frequency")
	scanLen := flag.Int("scan-len", dict.ScanLen, "scan window length")
	n := flag.Int("n", 10, "candidates to show")
	asJSON := flag.Bool("json", false, "print JSON")
	senses := flag.Int("senses", 2, "senses to show per candidate")
	dbPath := flag.String("db", "", "path to jmdict.sqlite")
	flag.Parse()
	if *dbPath == "" {
		*dbPath = findDB()
	}

	db, err := dict.Open(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()

	start := time.Now()
	var cands []dict.Candidate
	switch {
	case *scanText != "":
		text := []rune(*scanText)
		end := *offset
		for end < len(text) && text[end] != '\n' {
			end++ // the scan is bounded by the paragraph
		}
		cands, err = db.Scan(text, *offset, end, *scanLen, *policy, *n)
	case flag.NArg() == 1:
		cands, err = db.LookupCandidates(flag.Arg(0), *n)
	default:
		fmt.Fprintln(os.Stderr, "usage: lookup --scan TEXT [--offset N] | lookup SELECTION")
		os.Exit(2)
	}
	elapsed := time.Since(start)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", " ")
		enc.Encode(cands)
		return
	}
	if len(cands) == 0 {
		fmt.Println("(no match)")
	}
	for i, c := range cands {
		rank := "unranked"
		if c.Rank < dict.Unranked {
			rank = fmt.Sprintf("#%d", c.Rank)
		}
		note := ""
		if len(c.Reasons) > 0 {
			note = " ← " + strings.Join(c.Reasons, " ← ")
		}
		if c.CompletedFrom != nil {
			note += " (completed from " + *c.CompletedFrom + ")"
		}
		src := ""
		if c.SourceLen > 0 {
			src = fmt.Sprintf(" src=%d", c.SourceLen)
		}
		fmt.Printf("%2d. %s【%s】 %s%s%s  [%s]\n", i+1, c.Word, c.Reading, rank, src, note, strings.Join(c.POS, ","))
		for j, s := range c.Senses {
			if j >= *senses {
				break
			}
			fmt.Printf("      %d. %s\n", j+1, strings.Join(s.Glosses, "; "))
		}
	}
	fmt.Fprintf(os.Stderr, "(%v)\n", elapsed.Round(time.Microsecond))
}
