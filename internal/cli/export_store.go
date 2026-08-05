// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: extends the generated `export` command with local-store
// resources (eod, index, companies-local) for a stable downstream contract
// (issue #9). Wraps the generated RunE so live `export companies` stays intact.

package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ngpestelos/pse-edge-pp-cli/internal/store"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		var exp *cobra.Command
		for _, c := range root.Commands() {
			if c.Name() == "export" {
				exp = c
				break
			}
		}
		if exp == nil {
			return
		}
		attachStoreExport(exp, flags)
	})
}

func attachStoreExport(cmd *cobra.Command, flags *rootFlags) {
	var dbPath string
	var fromDate string
	var toDate string
	var symbolsFlag string
	var codesFlag string

	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (local resources eod/index/companies-local; default: data dir data.db)")
	cmd.Flags().StringVar(&fromDate, "from", "", "Range start YYYY-MM-DD (eod/index local export)")
	cmd.Flags().StringVar(&toDate, "to", "", "Range end YYYY-MM-DD (eod/index local export)")
	cmd.Flags().StringVar(&symbolsFlag, "symbols", "", "Comma-separated symbols (eod local export)")
	cmd.Flags().StringVar(&codesFlag, "codes", "", "Comma-separated index codes (index local export; default: all synced)")

	cmd.Short = "Export data as JSONL/JSON — live API or local-store market tables"
	cmd.Long = `Export paginated live API data or local SQLite market tables.

Live API (network):
  companies

Local store — stable downstream contract (prefer for analytics pipelines):
  eod              daily bars (pse-edge-export-eod-v1)
  index            index snapshots + breadth (pse-edge-export-index-v1)
  companies-local  registry rows (pse-edge-export-companies-v1)

Local exports are the supported integration boundary. Do not treat the
physical data.db schema as a public ABI. See docs/downstream-integration.md.

Date filters (--from/--to) are YYYY-MM-DD and apply only to eod/index.
JSONL is recommended for large datasets.`
	cmd.Example = `  # Local EOD bars for a date window
  pse-edge-pp-cli export eod --from 2025-01-01 --to 2026-07-27 --format jsonl -o eod.jsonl

  # Index snapshots (PSEi + sectors already synced)
  pse-edge-pp-cli export index --from 2025-01-01 --format jsonl

  # One or more symbols
  pse-edge-pp-cli export eod --symbols AT,GTCAP --from 2026-01-01 --format jsonl

  # Live API companies page stream
  pse-edge-pp-cli export companies --format jsonl --limit 100`

	orig := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if len(args) == 0 {
			return c.Help()
		}
		resource := args[0]
		switch resource {
		case "eod", "index", "companies-local":
			format, _ := c.Flags().GetString("format")
			outputFile, _ := c.Flags().GetString("output")
			limit, _ := c.Flags().GetInt("limit")
			return runStoreExport(c, flags, storeExportOpts{
				resource:   resource,
				format:     format,
				outputFile: outputFile,
				limit:      limit,
				dbPath:     dbPath,
				from:       fromDate,
				to:         toDate,
				symbols:    splitCSV(symbolsFlag),
				codes:      splitCSV(codesFlag),
			})
		default:
			// Generated path only knows "companies"; rewrite its error for new names.
			err := orig(c, args)
			if err != nil && strings.Contains(err.Error(), "unknown resource") {
				return usageErr(fmt.Errorf("unknown resource %q; valid: companies, companies-local, eod, index", resource))
			}
			return err
		}
	}
}

type storeExportOpts struct {
	resource   string
	format     string
	outputFile string
	limit      int
	dbPath     string
	from       string
	to         string
	symbols    []string
	codes      []string
}

func runStoreExport(cmd *cobra.Command, flags *rootFlags, opt storeExportOpts) error {
	if dryRunOK(flags) {
		return nil
	}
	format := strings.ToLower(strings.TrimSpace(opt.format))
	if format == "" {
		format = "jsonl"
	}
	if format != "jsonl" && format != "json" {
		return usageErr(fmt.Errorf("invalid --format %q (jsonl|json)", opt.format))
	}
	for _, d := range []struct{ name, val string }{{"--from", opt.from}, {"--to", opt.to}} {
		if d.val == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", d.val); err != nil {
			return usageErr(fmt.Errorf("invalid %s %q: expected YYYY-MM-DD", d.name, d.val))
		}
	}
	if opt.dbPath == "" {
		opt.dbPath = defaultDBPath("pse-edge-pp-cli")
	}
	db, err := store.OpenWithContext(cmd.Context(), opt.dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.EnsurePSEEdgeTables(cmd.Context()); err != nil {
		return err
	}

	var out io.Writer = cmd.OutOrStdout()
	var closer io.Closer
	if opt.outputFile != "" {
		f, err := os.Create(opt.outputFile)
		if err != nil {
			return err
		}
		closer = f
		out = f
	}
	defer func() {
		if closer != nil {
			_ = closer.Close()
		}
	}()
	w := bufio.NewWriter(out)
	defer w.Flush()

	var count int
	switch opt.resource {
	case "eod":
		count, err = writeExportStream(format, w, func(emit func(any) error) (int, error) {
			return db.StreamExportEOD(cmd.Context(), opt.from, opt.to, opt.symbols, opt.limit, func(r store.ExportEODRow) error {
				return emit(r)
			})
		})
	case "index":
		count, err = writeExportStream(format, w, func(emit func(any) error) (int, error) {
			return db.StreamExportIndex(cmd.Context(), opt.from, opt.to, opt.codes, opt.limit, func(r store.ExportIndexRow) error {
				return emit(r)
			})
		})
	case "companies-local":
		count, err = writeExportStream(format, w, func(emit func(any) error) (int, error) {
			return db.StreamExportCompanies(cmd.Context(), opt.limit, func(r store.ExportCompanyRow) error {
				return emit(r)
			})
		})
	default:
		return usageErr(fmt.Errorf("unknown local resource %q", opt.resource))
	}
	if err != nil {
		return err
	}

	if opt.outputFile != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "Exported %d records to %s (%s)\n", count, opt.outputFile, opt.resource)
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "note: exported %d %s record(s); each row includes a \"contract\" field naming the versioned schema\n", count, opt.resource)
	}
	return nil
}

func writeExportStream(format string, w *bufio.Writer, stream func(emit func(any) error) (int, error)) (int, error) {
	if format == "json" {
		var all []any
		n, err := stream(func(v any) error {
			all = append(all, v)
			return nil
		})
		if err != nil {
			return n, err
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return n, enc.Encode(all)
	}
	enc := json.NewEncoder(w)
	return stream(func(v any) error {
		return enc.Encode(v)
	})
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	return out
}
