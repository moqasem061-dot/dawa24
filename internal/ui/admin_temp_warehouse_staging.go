package ui

import (
	"context"
	"fmt"
	"math"
	"mime/multipart"
	"os"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Staging temp-warehouse uploads without holding the browser open.
//
// The handler this splits was written for "60-100+ files in parallel" and did
// every one of them inside the POST: read the bytes, parse the workbook, detect
// the columns, insert every row, then wait on all of it before answering. A
// hundred warehouse exports is minutes of work, and the route is not in
// httpx.longRunningPrefixes — so the request deadline cut it off, the browser
// showed a spinner until it did, and a batch that was half-written stayed
// half-written with nothing to say which half.
//
// Two phases now, the same split the compare tool and the vendor import use:
//
//	in the request    read the bytes, refuse anything that is not a
//	                  spreadsheet, write it to disk, create the row as
//	                  FileProcessing
//	in the background parse, detect the columns, insert the rows, mark it ready
//
// The bytes are NOT carried into the goroutine. A multipart temp file is
// removed the moment the handler returns, so they must be read now — but they
// are written to disk here and read back there, which is what keeps a
// hundred-file batch off the heap. resolveStoragePath already knows every place
// this application writes an upload, which is what makes that safe here and did
// not make it safe for the compare tool.

// tempWarehouseStageTimeout bounds one detached parse.
const tempWarehouseStageTimeout = 20 * time.Minute

// tempWarehouseStageWorkers bounds how many files are parsed at once.
//
// Four, where the synchronous version used sixteen. Each one holds a database
// connection while it inserts and the pool is twenty for the whole process, so
// sixteen parsers plus the requests they compete with is how a bulk upload
// takes the rest of the site down with it. The batch finishes at close to the
// same time either way: the work is bounded by the database, not by how many
// goroutines are queued in front of it.
const tempWarehouseStageWorkers = 4

// tempWarehouseStageQueue serialises staging across every batch in flight, so
// two admins uploading a hundred files each cannot start two hundred parses.
var tempWarehouseStageQueue = make(chan struct{}, tempWarehouseStageWorkers)

// registerTempWarehouseFile records one uploaded file and returns immediately.
//
// The row it creates is FileProcessing: its columns have not been read, so no
// screen may offer a mapping for it yet. stageTempWarehouseFile finishes the
// job on its own goroutine.
func (h *UIHandler) registerTempWarehouseFile(
	ctx context.Context,
	fh *multipart.FileHeader,
	defaultSupplierName string,
	customCode, customName, customPrice, customDiscount string,
	userID int64,
	orgID *int64,
) tempWarehouseUploadResult {
	fileBytes, res := readTempWarehouseUpload(fh)
	if !res.Success {
		return res
	}

	supplierName := defaultSupplierName
	if supplierName == "" {
		supplierName = cleanSupplierNameFromFilename(fh.Filename)
	}

	// On disk before the row exists, so a row can never point at nothing.
	storageKey, _ := saveUploadedBytes(fileBytes, fh.Filename, "temp_warehouses")
	if storageKey == "" {
		return tempWarehouseUploadResult{
			Filename: fh.Filename, SupplierName: supplierName, Success: false,
			Error: i18n.T("ar", "admin.temp_warehouse.insufficient_rows"),
		}
	}

	compareFile := &compare.CompareFile{
		UserID:           userID,
		OrganizationID:   orgID,
		SupplierName:     supplierName,
		OriginalFilename: fh.Filename,
		StorageKey:       storageKey,
		MIMEType:         "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		SizeBytes:        int64(len(fileBytes)),
		Status:           compare.FileProcessing,
		IsTempWarehouse:  true,
	}
	if err := h.compareSvc.CreateFile(database.AsSystem(ctx), compareFile); err != nil {
		return tempWarehouseUploadResult{
			Filename: fh.Filename, SupplierName: supplierName, Success: false,
			Error: fmt.Sprintf(i18n.T("ar", "admin.temp_warehouse.create_record_failed_format"), err.Error()),
		}
	}

	// Detached: keeps the request's identity, loses its cancellation, so
	// closing the tab no longer abandons a half-parsed batch.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tempWarehouseStageTimeout)
	staged := *compareFile

	go func() {
		defer cancel()
		tempWarehouseStageQueue <- struct{}{}
		defer func() { <-tempWarehouseStageQueue }()
		defer func() {
			if p := recover(); p != nil {
				h.log.ErrorContext(runCtx, "temp warehouse staging panicked",
					"file", staged.ID, "panic", p)
				h.failTempWarehouse(runCtx, &staged, i18n.T("ar", "admin.temp_warehouse.insufficient_rows"))
			}
		}()
		h.stageTempWarehouseFile(runCtx, &staged, customCode, customName, customPrice, customDiscount)
	}()

	return tempWarehouseUploadResult{
		ID:           compareFile.ID,
		Filename:     fh.Filename,
		SupplierName: supplierName,
		Success:      true,
	}
}

// readTempWarehouseUpload pulls the bytes out of the multipart part and refuses
// anything that is not a readable spreadsheet.
//
// Synchronous on purpose: a file that is not a spreadsheet at all should be
// rejected while somebody is still looking at the screen that rejected it, and
// the multipart temp file is deleted the moment the handler returns.
func readTempWarehouseUpload(fh *multipart.FileHeader) ([]byte, tempWarehouseUploadResult) {
	file, err := fh.Open()
	if err != nil {
		return nil, tempWarehouseUploadResult{Filename: fh.Filename, Success: false,
			Error: fmt.Sprintf(i18n.T("ar", "admin.temp_warehouse.open_failed_format"), err.Error())}
	}
	defer file.Close()

	fileBytes, err := readAllUpTo(file, maxImportBatchBytes)
	if err != nil {
		return nil, tempWarehouseUploadResult{Filename: fh.Filename, Success: false,
			Error: fmt.Sprintf(i18n.T("ar", "admin.temp_warehouse.read_failed_format"), err.Error())}
	}
	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, fh.Filename); err != nil {
		return nil, tempWarehouseUploadResult{Filename: fh.Filename, Success: false,
			Error: filesecurity.SecurityErrorMessage}
	}
	return fileBytes, tempWarehouseUploadResult{Success: true}
}

// stageTempWarehouseFile parses one registered file and stores its rows.
func (h *UIHandler) stageTempWarehouseFile(
	ctx context.Context,
	file *compare.CompareFile,
	customCode, customName, customPrice, customDiscount string,
) {
	path := resolveStoragePath(file.StorageKey, "temp_warehouses")
	if path == "" {
		h.failTempWarehouse(ctx, file, i18n.T("ar", "admin.temp_warehouse.insufficient_rows"))
		return
	}
	fileBytes, err := os.ReadFile(path)
	if err != nil || len(fileBytes) == 0 {
		h.failTempWarehouse(ctx, file, i18n.T("ar", "admin.temp_warehouse.insufficient_rows"))
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, file.OriginalFilename)
	if err != nil || len(rawRows) < 2 {
		msg := i18n.T("ar", "admin.temp_warehouse.insufficient_rows")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		h.failTempWarehouse(ctx, file, msg)
		return
	}

	// Row zero is not reliably the header row. Warehouse exports routinely
	// carry a title band above it — the warehouse's name, an export date, a
	// blank spacer — and reading that as the header bound the code column to a
	// price. AnalyzeLayout is what every other importer uses.
	layout, _ := productmatch.AnalyzeLayout(rawRows)
	headers := layout.Headers
	dataStart := layout.FirstDataRow
	if layout.HeaderRow < 0 || len(headers) == 0 {
		headers = rawRows[0]
		dataStart = 1
	}
	if dataStart < 1 || dataStart > len(rawRows) {
		dataStart = 1
	}
	dataRows := rawRows[dataStart:]

	codeCol, nameCol, priceCol, discountCol := detectTempWarehouseCols(headers, customCode, customName, customPrice, customDiscount)
	file.MappingConfig = compare.MappingConfig{
		CodeCol: &codeCol, NameCol: &nameCol, PriceCol: &priceCol, DiscountCol: &discountCol,
	}

	fileRows := buildTempWarehouseRows(file, dataRows, dataStart, codeCol, nameCol, priceCol, discountCol)
	if len(fileRows) > 0 {
		if err := h.compareSvc.InsertFileRows(database.AsSystem(ctx), fileRows); err != nil {
			h.log.ErrorContext(ctx, "insert warehouse file rows", "error", err, "file_id", file.ID)
			h.failTempWarehouse(ctx, file, i18n.T("ar", "admin.temp_warehouse.insufficient_rows"))
			return
		}
	}

	file.RowCount = len(fileRows)
	file.Status = compare.FileReady
	file.ErrorMessage = ""
	if err := h.compareSvc.UpdateFile(database.AsSystem(ctx), file); err != nil {
		h.log.ErrorContext(ctx, "temp warehouse: record completion", "error", err, "file_id", file.ID)
	}
}

// buildTempWarehouseRows turns the sheet's data rows into storable rows.
func buildTempWarehouseRows(
	file *compare.CompareFile, dataRows [][]string, dataStart int,
	codeCol, nameCol, priceCol, discountCol int,
) []*compare.CompareFileRow {
	out := make([]*compare.CompareFileRow, 0, len(dataRows))
	for idx, row := range dataRows {
		if len(row) == 0 {
			continue
		}
		rawName := ""
		if nameCol >= 0 && nameCol < len(row) {
			rawName = strings.TrimSpace(row[nameCol])
		}
		if rawName == "" {
			continue
		}
		sku := ""
		if codeCol >= 0 && codeCol < len(row) {
			sku = strings.TrimSpace(row[codeCol])
		}

		priceMinor := int64(0)
		if priceCol >= 0 && priceCol < len(row) {
			if p, err := parsePriceFloat(row[priceCol]); err == nil && p > 0 {
				priceMinor = int64(math.Round(p * 100))
			}
		}
		discountPct := 0.0
		if discountCol >= 0 && discountCol < len(row) {
			if d, err := parsePriceFloat(row[discountCol]); err == nil && d >= 0 {
				discountPct = math.Min(d, 100)
			}
		}
		priceAfterMinor := int64(math.Round(float64(priceMinor) * (1.0 - (discountPct / 100.0))))

		out = append(out, &compare.CompareFileRow{
			FileID:             file.ID,
			OrganizationID:     file.OrganizationID,
			RowNumber:          dataStart + idx + 1,
			RawName:            rawName,
			NormalizedName:     strings.ToLower(rawName),
			SKU:                sku,
			Price:              money.FromMinor(priceMinor),
			Discount:           discountPct,
			PriceAfterDiscount: money.FromMinor(priceAfterMinor),
		})
	}
	return out
}

// failTempWarehouse records that a staging pass ended badly, so the screen
// stops waiting and says why.
func (h *UIHandler) failTempWarehouse(ctx context.Context, file *compare.CompareFile, message string) {
	file.Status = compare.FileFailed
	file.ErrorMessage = message
	if err := h.compareSvc.UpdateFile(database.AsSystem(ctx), file); err != nil {
		h.log.ErrorContext(ctx, "temp warehouse: record failure", "error", err, "file_id", file.ID)
	}
}
