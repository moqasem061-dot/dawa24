package assistant

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/datasets"
	"github.com/muhiya/dawa24-store/internal/shared/timeutil"
)

// Exports: the answer to "give me all of it".
//
// A reply can carry a few hundred rows before it stops being readable; a
// spreadsheet can carry fifty thousand. export_data runs the same governed
// query and hands the user a file. The file is stored against its owner with a
// random token (only the token's hash is kept) and expires; the web drawer
// downloads it with the session, the Telegram bridge sends it as a document.

// ExportTTL is how long an export stays downloadable.
const ExportTTL = 7 * 24 * 60 * 60 // seconds

// MaxExportBytes bounds one stored file.
const MaxExportBytes = 20 << 20

// ExportFile is a generated file before it is stored.
type ExportFile struct {
	Filename string
	MIMEType string
	Rows     int
	Content  []byte
}

// Export is a stored file as its owner reads it back.
type Export struct {
	ID             int64
	OrganizationID int64
	UserID         int64
	ExportFile
}

// Entity kinds that are not dashboard records.
const (
	// EntityExport is a downloadable file the turn produced.
	EntityExport EntityKind = "export"
	// EntityProposal is a proposed action awaiting the user's confirmation.
	EntityProposal EntityKind = "action"
)

const (
	mimeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	mimeCSV  = "text/csv; charset=utf-8"
)

// ProposalCard is a pending action as the drawer and Telegram render it. It
// carries the public id and the server-rendered preview — never the arguments.
type ProposalCard struct {
	ID        string           `json:"id"`
	Action    string           `json:"action"`
	Risk      string           `json:"risk"`
	Status    string           `json:"status"`
	Preview   actions.Preview  `json:"preview"`
	Outcome   *actions.Outcome `json:"outcome,omitempty"`
	Error     string           `json:"error,omitempty"`
	ExpiresAt time.Time        `json:"expires_at"`
}

// ProposalEntity presents a pending action under the answer.
func ProposalEntity(p *actions.Pending) Entity {
	return Entity{
		Kind:     EntityProposal,
		ID:       p.ID,
		Label:    p.Preview.Title,
		Title:    p.Preview.Title,
		Subtitle: p.Preview.Summary,
		Proposal: &ProposalCard{
			ID: p.PublicID.String(), Action: p.Action, Risk: string(p.Risk), Status: string(p.Status),
			Preview: p.Preview, Outcome: p.Outcome, Error: p.Error, ExpiresAt: p.ExpiresAt,
		},
	}
}

// ExportPath is the site-relative download path for a token.
func ExportPath(token string) string { return "/api/v1/assistant/exports/" + token }

// ExportEntity presents a stored export as a reference under the answer.
func ExportEntity(f ExportFile, token string) Entity {
	return Entity{
		Kind:     EntityExport,
		ID:       tokenID(token),
		Label:    f.Filename,
		Title:    f.Filename,
		Subtitle: fmt.Sprintf("%d صف", f.Rows),
		URL:      ExportPath(token),
	}
}

// tokenID gives an export entity a stable id for de-duplication. It is not a
// database id and nothing looks a record up by it.
func tokenID(token string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(token))
	return int64(h.Sum64() >> 1)
}

var unsafeFilename = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)

// BuildExport renders columns and rows into a file.
func BuildExport(title, format string, cols []datasets.Column, rows [][]any) (ExportFile, error) {
	base := strings.TrimSpace(unsafeFilename.ReplaceAllString(title, " "))
	if base == "" {
		base = "export"
	}
	base += " " + timeutil.Now().Format("2006-01-02")

	var (
		out ExportFile
		err error
	)
	switch format {
	case "csv":
		out, err = buildCSV(cols, rows)
		out.Filename = base + ".csv"
	default:
		out, err = buildXLSX(cols, rows)
		out.Filename = base + ".xlsx"
	}
	if err != nil {
		return ExportFile{}, err
	}
	if len(out.Content) > MaxExportBytes {
		return ExportFile{}, fmt.Errorf("assistant export: %d bytes exceeds the %d byte ceiling", len(out.Content), MaxExportBytes)
	}
	out.Rows = len(rows)
	return out, nil
}

func buildCSV(cols []datasets.Column, rows [][]any) (ExportFile, error) {
	var buf bytes.Buffer
	buf.WriteString("\ufeff") // Excel reads UTF-8 Arabic only with a BOM.
	w := csv.NewWriter(&buf)
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = c.Label
	}
	if err := w.Write(header); err != nil {
		return ExportFile{}, err
	}
	record := make([]string, len(cols))
	for _, row := range rows {
		for i, v := range row {
			record[i] = csvCell(v)
		}
		if err := w.Write(record); err != nil {
			return ExportFile{}, err
		}
	}
	w.Flush()
	return ExportFile{MIMEType: mimeCSV, Content: buf.Bytes()}, w.Error()
}

// csvCell neutralises spreadsheet formulas. A product name a supplier typed as
// "=HYPERLINK(...)" must open as text, not run.
func csvCell(v any) string {
	s := cellText(v)
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		if _, isNum := v.(json.Number); !isNum {
			return "'" + s
		}
	}
	return s
}

func cellText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "نعم"
		}
		return "لا"
	}
	return fmt.Sprint(v)
}

func buildXLSX(cols []datasets.Column, rows [][]any) (ExportFile, error) {
	f := excelize.NewFile()
	defer f.Close()
	const sheet = "Sheet1"
	rtl := true
	if err := f.SetSheetView(sheet, 0, &excelize.ViewOptions{RightToLeft: &rtl}); err != nil {
		return ExportFile{}, err
	}
	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		return ExportFile{}, err
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return ExportFile{}, err
	}
	header := make([]any, len(cols))
	for i, c := range cols {
		header[i] = excelize.Cell{StyleID: bold, Value: c.Label}
	}
	if err := sw.SetRow("A1", header); err != nil {
		return ExportFile{}, err
	}
	for r, row := range rows {
		cells := make([]any, len(row))
		for i, v := range row {
			cells[i] = xlsxCell(cols[i].Type, v)
		}
		cell, err := excelize.CoordinatesToCellName(1, r+2)
		if err != nil {
			return ExportFile{}, err
		}
		if err := sw.SetRow(cell, cells); err != nil {
			return ExportFile{}, err
		}
	}
	if err := sw.Flush(); err != nil {
		return ExportFile{}, err
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return ExportFile{}, err
	}
	return ExportFile{MIMEType: mimeXLSX, Content: buf.Bytes()}, nil
}

// xlsxCell writes numbers as numbers so the sheet can be summed, and every
// string as a string: excelize's stream writer never evaluates text as a
// formula, so a hostile product name stays inert.
func xlsxCell(t datasets.Type, v any) any {
	switch val := v.(type) {
	case nil:
		return nil
	case json.Number:
		if t == datasets.Int {
			if n, err := val.Int64(); err == nil {
				return n
			}
		}
		if f, err := val.Float64(); err == nil {
			return f
		}
		return val.String()
	case bool:
		return cellText(val)
	}
	return v
}
