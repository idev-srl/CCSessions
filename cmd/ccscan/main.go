// Command ccscan prints the directories and sessions found by ccdata.
// It is a diagnostic tool; it never writes anything.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/idev-srl/CCSessions/internal/ccdata"
)

func main() {
	verbose := flag.Bool("v", false, "also list the sessions")
	asJSON := flag.Bool("json", false, "print the scan result as JSON")
	flag.Parse()

	loc := ccdata.DefaultLocations()
	sc := ccdata.NewScanner(loc)
	res := sc.Scan()
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	fmt.Printf("base=%s claude.json=%s  %d directories in %d ms\n", loc.Base, loc.ClaudeJSON, len(res.Directories), res.DurationMs)
	for _, d := range res.Directories {
		state := "ok"
		if !d.Exists {
			state = "MISSING"
		}
		last := "-"
		if !d.LastSession.IsZero() {
			last = d.LastSession.Local().Format("2006-01-02 15:04")
		}
		fmt.Printf("%-16s %3d sess %8.1f MB %-8s json=%-5v %s\n", last, d.SessionCount, float64(d.TotalSize)/1e6, state, d.InClaudeJSON, d.Path)
		if *verbose {
			for _, s := range d.Sessions {
				fmt.Printf("    %s %s u=%d a=%d %s\n", s.ID[:8], s.LastEvent.Local().Format("01-02 15:04"), s.UserMessages, s.AssistantMessages, s.Preview)
			}
		}
	}
	for _, e := range res.Errors {
		fmt.Println("ERR", e)
	}
	days, found, err := ccdata.ReadCleanupDays(loc.SettingsFile())
	fmt.Println("cleanupPeriodDays:", days, found, err)
	p := ccdata.PreviewCleanup(res.AllSessions(), ccdata.DefaultCleanupDays, time.Now())
	fmt.Printf("preview 30d: %+v\n", p)
	fmt.Printf("overrides: %+v\n", ccdata.FindOverrides(loc, nil))
	t0 := time.Now()
	sc.Scan()
	fmt.Println("rescan (cache):", time.Since(t0))
}
