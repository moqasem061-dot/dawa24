package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matcheval"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func runMatchCorpusExport(args []string) int {
	fs := flag.NewFlagSet("match-corpus-export", flag.ContinueOnError)
	workflow := fs.String("workflow", "", "Target workflow (vendor|savings|smart_order|catalog|compare)")
	since := fs.String("since", "", "Filter rows since date (YYYY-MM-DD)")
	limit := fs.Int("limit", 5000, "Maximum number of cases to export")
	outDir := fs.String("out", "", "Output directory outside the repository root (required)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-export: parse flags: %v\n", err)
		return 1
	}

	if *outDir == "" {
		fmt.Fprintf(os.Stderr, "match-corpus-export: --out is required\n")
		return 1
	}

	// Verify that outDir is strictly OUTSIDE the repository root (R17, leakage control)
	repoRoot, err := filepath.Abs(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-export: could not determine repo root: %v\n", err)
		return 1
	}
	outAbs, err := filepath.Abs(*outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-export: invalid output path: %v\n", err)
		return 1
	}

	rel, err := filepath.Rel(repoRoot, outAbs)
	if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		fmt.Fprintf(os.Stderr, "match-corpus-export: --out path (%s) MUST NOT be under repository root (%s) for privacy/leakage control\n", outAbs, repoRoot)
		return 1
	}

	if err := os.MkdirAll(outAbs, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-export: create output dir: %v\n", err)
		return 1
	}

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-export: load config: %v\n", err)
		return 1
	}

	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-export: database connect: %v\n", err)
		return 1
	}
	defer db.Close()

	casesPath := filepath.Join(outAbs, "cases.jsonl")
	f, err := os.Create(casesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-export: create cases file: %v\n", err)
		return 1
	}
	defer f.Close()

	exported := 0
	enc := json.NewEncoder(f)

	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Export from ingest.catalog_import_rows (Vendor)
		if *workflow == "" || *workflow == "vendor" {
			n, err := exportVendorRows(txCtx, tx, enc, *since, *limit-exported)
			if err != nil {
				return fmt.Errorf("vendor export: %w", err)
			}
			exported += n
		}

		// 2. Export from smartorder.run_lines (Smart Order)
		if (*workflow == "" || *workflow == "smart_order") && exported < *limit {
			n, err := exportSmartOrderLines(txCtx, tx, enc, *since, *limit-exported)
			if err != nil {
				return fmt.Errorf("smartorder export: %w", err)
			}
			exported += n
		}

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-export: export failed: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully exported %d de-identified cases to %s\n", exported, casesPath)
	return 0
}

func exportVendorRows(ctx context.Context, tx pgx.Tx, enc *json.Encoder, since string, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}

	query := `
		SELECT r.id, r.import_id, r.organization_id, r.display_name, r.source_code, r.product_id, r.payload, i.filename
		FROM ingest.catalog_import_rows r
		JOIN ingest.catalog_imports i ON i.id = r.import_id
	`
	var args []any
	if since != "" {
		query += " WHERE r.created_at >= $1 ORDER BY r.id LIMIT $2"
		args = append(args, since, limit)
	} else {
		query += " ORDER BY r.id LIMIT $1"
		args = append(args, limit)
	}

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			rowID        int64
			importID     int64
			orgID        int64
			displayName  string
			sourceCode   string
			productID    *int64
			payloadBytes []byte
			filename     string
		)
		if err := rows.Scan(&rowID, &importID, &orgID, &displayName, &sourceCode, &productID, &payloadBytes, &filename); err != nil {
			return count, err
		}

		c := buildDeidentifiedCase("vendor", rowID, importID, orgID, displayName, sourceCode, productID, payloadBytes, filename)
		if err := enc.Encode(c); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func exportSmartOrderLines(ctx context.Context, tx pgx.Tx, enc *json.Encoder, since string, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}

	query := `
		SELECT l.id, l.run_id, r.organization_id, l.raw_text, COALESCE(s.product_id, 0)
		FROM smartorder.run_lines l
		JOIN smartorder.runs r ON r.id = l.run_id
		LEFT JOIN smartorder.line_selections s ON s.line_id = l.id
	`
	var args []any
	if since != "" {
		query += " WHERE l.created_at >= $1 ORDER BY l.id LIMIT $2"
		args = append(args, since, limit)
	} else {
		query += " ORDER BY l.id LIMIT $1"
		args = append(args, limit)
	}

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			lineID    int64
			runID     int64
			orgID     int64
			rawText   string
			productID int64
		)
		if err := rows.Scan(&lineID, &runID, &orgID, &rawText, &productID); err != nil {
			return count, err
		}

		var pID *int64
		if productID > 0 {
			pID = &productID
		}
		c := buildDeidentifiedCase("smart_order", lineID, runID, orgID, rawText, "", pID, nil, "smartorder.csv")
		if err := enc.Encode(c); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func buildDeidentifiedCase(
	workflow string,
	rowID, parentID, orgID int64,
	name, code string,
	productID *int64,
	payloadBytes []byte,
	filename string,
) matcheval.Case {
	cells := make(map[string]string)
	if len(payloadBytes) > 0 {
		var raw map[string]any
		if err := json.Unmarshal(payloadBytes, &raw); err == nil {
			for k, v := range raw {
				kLower := strings.ToLower(k)
				// De-identify: drop private columns (org, price, cost, quantity)
				if strings.Contains(kLower, "org") || strings.Contains(kLower, "price") ||
					strings.Contains(kLower, "cost") || strings.Contains(kLower, "qty") ||
					strings.Contains(kLower, "quantity") || strings.Contains(kLower, "total") ||
					strings.Contains(kLower, "amount") {
					continue
				}
				cells[kLower] = fmt.Sprint(v)
			}
		}
	}
	if cells["name"] == "" && name != "" {
		cells["name"] = name
	}

	var identifiers []matcheval.CaseIdentifier
	if code != "" {
		// Namespace-preserving surrogate: hash(orgID:code)
		surrogate := fmt.Sprintf("sc_%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%s", orgID, code))))[:12]
		cells["code"] = surrogate
		identifiers = append(identifiers, matcheval.CaseIdentifier{
			Column:    "code",
			Namespace: "supplier_code",
		})
	}

	// Compute template family = hash(orgID:filename)
	tfHash := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%s", orgID, filename))))[:10]
	templateFamily := "tf-" + tfHash

	// Compute expression family = hash(QuestionSignature)
	rowFacts := productmatch.ExtractRowFacts(&productmatch.Row{Name: cells["name"]}, nil)
	sig := productmatch.QuestionSignature(rowFacts, cells["name"], productmatch.NormalizerVersion)
	efHash := fmt.Sprintf("%x", sha256.Sum256([]byte(sig)))[:10]
	expressionFamily := "ef-" + efHash

	// Label construction
	resolvable := productID != nil && *productID > 0
	var targets []int64
	expect := "abstain"
	if resolvable {
		targets = []int64{*productID}
		expect = "accept"
	}

	return matcheval.Case{
		ID:                  fmt.Sprintf("c-%s-%d", workflow, rowID),
		Split:               "unassigned",
		TemplateFamily:      templateFamily,
		ExpressionFamily:    expressionFamily,
		Workflow:            []string{workflow},
		Language:            detectCaseLanguage(cells["name"]),
		Row:                 matcheval.CaseRow{Cells: cells, Identifiers: identifiers},
		RequiredGranularity: "product",
		Label: matcheval.Label{
			Resolvable:  resolvable,
			Targets:     targets,
			Granularity: "product",
			Expect:      expect,
			Labellers:   1,
		},
	}
}

func detectCaseLanguage(text string) string {
	hasAR := false
	hasEN := false
	for _, r := range text {
		if unicode.In(r, unicode.Arabic) {
			hasAR = true
		} else if unicode.IsLetter(r) && (r < 0x0590 || r > 0x08FF) {
			hasEN = true
		}
	}
	if hasAR && hasEN {
		return "mixed"
	}
	if hasAR {
		return "ar"
	}
	return "en"
}
