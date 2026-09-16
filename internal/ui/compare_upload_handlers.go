package ui

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// CompareSampleDownload generates and streams a realistic Egyptian pharmaceutical pricing & discount template file (.xlsx).
func (h *UIHandler) CompareSampleDownload(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := i18n.T("ar", "excel.sheet.price_list")
	f.SetSheetName("Sheet1", sheetName)

	// Set right-to-left layout for Arabic
	_ = f.SetSheetView(sheetName, 0, &excelize.ViewOptions{
		RightToLeft: func() *bool { b := true; return &b }(),
	})

	// Header style
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold:  true,
			Color: "#FFFFFF",
			Size:  11,
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#0F172A"},
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
	})

	headers := []string{i18n.TDefault("w4_ui.s_52_52"), i18n.TDefault("w4_ui.s_53_53"), i18n.TDefault("w4_ui.s_54_54"), i18n.TDefault("w4_ui.s_55_55"), i18n.TDefault("w4_ui.s_56_56")}
	for i, hName := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheetName, cell, hName)
		_ = f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}

	// 10 realistic pharmaceutical sample records
	samples := [][]any{
		{"1001", i18n.TDefault("w4_ui.24_57"), 45.00, 18.5, i18n.T("ar", "sample.notes.large_stock")},
		{"1002", i18n.TDefault("w4_ui.1_14_58"), 135.00, 12.0, i18n.T("ar", "sample.notes.extra_discount")},
		{"1003", i18n.TDefault("w4_ui.20_59"), 31.00, 20.0, i18n.T("ar", "sample.notes.seasonal_offer")},
		{"1004", i18n.TDefault("w4_ui.50_20_60"), 58.50, 15.0, i18n.T("ar", "sample.notes.fresh_expiry")},
		{"1005", i18n.TDefault("w4_ui.20_61"), 22.00, 25.0, i18n.T("ar", "sample.notes.highest_discount")},
		{"1006", i18n.TDefault("w4_ui.400_30_62"), 48.00, 14.5, i18n.T("ar", "sample.notes.fast_delivery")},
		{"1007", i18n.TDefault("w4_ui.20_63"), 35.00, 16.0, i18n.TDefault("w4_ui.2027_17")},
		{"1008", i18n.TDefault("w4_ui.500_20_64"), 18.00, 22.5, i18n.T("ar", "sample.notes.pharmacy_offer")},
		{"1009", i18n.TDefault("w4_ui.500_3_65"), 52.00, 15.0, i18n.T("ar", "sample.notes.factory_direct")},
		{"1010", i18n.TDefault("w4_ui.20_66"), 28.00, 19.0, i18n.T("ar", "sample.notes.free_shipping")},
	}

	for rowIdx, row := range samples {
		for colIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			_ = f.SetCellValue(sheetName, cell, val)
		}
	}

	_ = f.SetColWidth(sheetName, "A", "A", 18)
	_ = f.SetColWidth(sheetName, "B", "B", 46)
	_ = f.SetColWidth(sheetName, "C", "C", 20)
	_ = f.SetColWidth(sheetName, "D", "D", 22)
	_ = f.SetColWidth(sheetName, "E", "E", 32)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_supplier_template.xlsx\"")
	_ = f.Write(w)
}

// CompareUploadSubmit handles uploading one or multiple supplier spreadsheet files and automatically parses rows in parallel.
func (h *UIHandler) CompareUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	var targetOrgID int64
	if param := chi.URLParam(r, "orgID"); param != "" {
		targetOrgID, _ = strconv.ParseInt(param, 10, 64)
	}
	if targetOrgID <= 0 {
		targetOrgID, _ = strconv.ParseInt(r.FormValue("org_id"), 10, 64)
	}

	isAdminMode := (actor.IsStaff || actor.IsPlatformAdmin()) && targetOrgID > 0
	returnURL := "/compare/tool"
	if isAdminMode {
		returnURL = fmt.Sprintf("/admin/organizations/import/%d/compare", targetOrgID)
	}

	if h.compareSvc == nil {
		h.redirectWithNotice(w, r, returnURL, "error", i18n.T(lang, "common.compare_service_unavailable"))
		return
	}

	// 128 MB max memory for multi-file batch uploads
	if err := parseImportUpload(w, r); err != nil {
		h.log.ErrorContext(ctx, "failed to parse compare upload", "error", err)
		h.redirectWithNotice(w, r, returnURL, "error", i18n.T(lang, "compare.upload.read_failed"))
		return
	}

	var fileHeaders []*multipart.FileHeader
	if fhs, ok := r.MultipartForm.File["compare_files"]; ok && len(fhs) > 0 {
		fileHeaders = fhs
	} else if fhs, ok := r.MultipartForm.File["compare_file"]; ok && len(fhs) > 0 {
		fileHeaders = fhs
	} else if fhs, ok := r.MultipartForm.File["files"]; ok && len(fhs) > 0 {
		fileHeaders = fhs
	} else if fhs, ok := r.MultipartForm.File["file"]; ok && len(fhs) > 0 {
		fileHeaders = fhs
	}

	if len(fileHeaders) == 0 {
		h.redirectWithNotice(w, r, returnURL, "error", i18n.T(lang, "compare.upload.choose_file"))
		return
	}

	var orgPtr *int64
	effectiveUserID := actor.UserID
	if isAdminMode {
		orgPtr = &targetOrgID
		if h.orgSvc != nil {
			if targetOrg, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), targetOrgID); err == nil && targetOrg != nil && targetOrg.OwnerID > 0 {
				effectiveUserID = targetOrg.OwnerID
			}
		}
	} else if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	sysCtx := ctx
	if isAdminMode {
		sysCtx = database.WithTenant(database.AsSystem(ctx), targetOrgID)
	}

	// Enforce compare files quota for File Center storage based on active subscription plan
	maxAllowedFiles := 10
	if h.billSvc != nil {
		if plan, err := h.billSvc.GetEffectivePlan(sysCtx, effectiveUserID, orgPtr); err == nil && plan != nil {
			maxAllowedFiles = plan.GetMaxCompareFiles()
		}
	}

	// A batch bigger than the plan allows takes what fits and names the rest.
	// It must not be refused outright: the previous lists are archived
	// automatically, so telling the vendor to clear them by hand asks for work
	// this upload already does.
	fileHeaders, quotaSkipped := trimToPlanQuota(fileHeaders, maxAllowedFiles, lang)

	type fileItem struct {
		index        int
		filename     string
		supplierName string
		contentType  string
		size         int64
		scanned      filesecurity.Scanned
		localURL     string
	}

	type fileResult struct {
		index    int
		file     *compare.CompareFile
		archived []string
		err      error
		errFile  string
	}

	// 1. Read and validate all uploaded file payloads into memory/disk
	var validItems []fileItem
	errorFiles := quotaSkipped

	for idx, header := range fileHeaders {
		if !SupportedUploadName(header.Filename) {
			errorFiles = append(errorFiles, header.Filename+" ("+i18n.T(lang, "compare.upload.unsupported_format")+")")
			continue
		}

		file, err := header.Open()
		if err != nil {
			errorFiles = append(errorFiles, header.Filename+" ("+i18n.T(lang, "compare.upload.open_failed")+")")
			continue
		}

		fileBytes, err := io.ReadAll(file)
		file.Close()
		if err != nil || len(fileBytes) == 0 {
			errorFiles = append(errorFiles, header.Filename+" ("+i18n.T(lang, "compare.upload.empty_or_unread")+")")
			continue
		}

		// Scanned once, here, before a byte of it reaches the disk. The proof
		// travels with the payload so the staging service cannot scan it again
		// and cannot forget to.
		scanned, err := filesecurity.Scan(fileBytes, header.Filename)
		if err != nil {
			errorFiles = append(errorFiles, header.Filename+" ("+filesecurity.SecurityErrorMessage+")")
			continue
		}

		supplierName := strings.TrimSpace(r.FormValue("supplier_name"))
		if supplierName == "" || len(fileHeaders) > 1 {
			supplierName = strings.TrimSpace(strings.TrimSuffix(header.Filename,
				filepath.Ext(header.Filename)))
			supplierName = strings.ReplaceAll(supplierName, "_", " ")
			supplierName = strings.ReplaceAll(supplierName, "-", " ")
		}
		if supplierName == "" {
			supplierName = header.Filename
		}

		localURL, localErr := saveUploadedBytes(fileBytes, header.Filename, "compare")
		if localErr != nil {
			h.log.ErrorContext(ctx, "failed to save uploaded compare file to disk", "error", localErr, "file", header.Filename)
			errorFiles = append(errorFiles, header.Filename+" ("+i18n.T(lang, "compare.upload.save_failed")+")")
			continue
		}

		validItems = append(validItems, fileItem{
			index:        idx,
			filename:     header.Filename,
			supplierName: supplierName,
			contentType:  header.Header.Get("Content-Type"),
			size:         header.Size,
			scanned:      scanned,
			localURL:     localURL,
		})
	}

	if len(validItems) == 0 {
		errMsg := i18n.T(lang, "compare.upload.none_processed_prefix") + strings.Join(errorFiles, ", ")
		h.redirectWithNotice(w, r, returnURL, "error", errMsg)
		return
	}

	// 2. Identify the exact oldest files to supersede only if incoming items exceed
	// remaining space under the subscription limit. If space remains, no files
	// will be archived. Actual archiving is deferred until this batch stages successfully.
	previousIDs := h.supersededFileIDs(sysCtx, effectiveUserID, orgPtr, len(validItems), maxAllowedFiles)

	// 3. Process valid files with bounded parallel concurrency.
	results := make([]fileResult, len(validItems))
	if len(validItems) > 0 {
		numWorkers := 6
		if len(validItems) < numWorkers {
			numWorkers = len(validItems)
		}

		itemChan := make(chan fileItem, len(validItems))
		var wg sync.WaitGroup

		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						h.log.Error("compare upload worker panicked", "panic", r)
					}
				}()
				for itm := range itemChan {
					// Registers the file and returns; the parse happens in a
					// goroutine that outlives this request. What used to happen
					// here — reading a whole workbook and writing every row of
					// it, for up to ten files — is why this endpoint had to be
					// exempted from the request deadline in the first place.
					staged, err := h.compareSvc.RegisterAndStage(
						sysCtx, effectiveUserID, orgPtr, itm.supplierName, itm.filename,
						itm.contentType, itm.size, itm.localURL, itm.scanned,
					)
					res := fileResult{index: itm.index, err: err}
					if staged != nil {
						res.file = staged.File
						res.archived = staged.Archived
					}
					if err != nil {
						res.errFile = itm.filename + " (" + h.safeMessage(err, lang) + ")"
					}
					results[itm.index] = res
				}
			}()
		}

		for i, itm := range validItems {
			itm.index = i
			itemChan <- itm
		}
		close(itemChan)
		wg.Wait()
	}

	// 4. Aggregate results
	var processedCount int
	var totalRows int
	var uploadedIDs []string
	var stagedIDs []int64

	for _, res := range results {
		if res.err != nil {
			if res.errFile != "" {
				errorFiles = append(errorFiles, res.errFile)
			}
			continue
		}
		if res.file != nil {
			processedCount++
			totalRows += res.file.RowCount
			uploadedIDs = append(uploadedIDs, strconv.FormatInt(res.file.ID, 10))
			stagedIDs = append(stagedIDs, res.file.ID)
		}
	}

	if processedCount == 0 {
		errMsg := i18n.T(lang, "compare.upload.none_processed_prefix") + strings.Join(errorFiles, ", ")
		h.redirectWithNotice(w, r, returnURL, "error", errMsg)
		return
	}

	// The rows have not been read yet — that is the point — so the summary
	// counts files, not rows. Claiming "0 rows" for a batch still being parsed
	// is worse than not mentioning rows at all.
	msg := fmt.Sprintf(i18n.T(lang, "compare.upload.staging_summary"), processedCount)

	// 5. Retire the generation this batch replaces — but only once the batch is
	// staged. The supervisor outlives this request and archives nothing if
	// every new file fails to parse.
	if len(previousIDs) > 0 {
		h.compareSvc.ReplacePreviousFiles(sysCtx, previousIDs, stagedIDs,
			i18n.T("ar", "compare.upload.replace_reason"))
		msg += fmt.Sprintf(i18n.T(lang, "compare.upload.replace_pending"), len(previousIDs))
	}
	_ = totalRows
	firstID := uploadedIDs[0]
	queueStr := strings.Join(uploadedIDs, ",")
	redirectURL := fmt.Sprintf("%s?setup_queue=%s&setup_file=%s&setup_step=1&setup_total=%d&notice=success&msg=%s",
		returnURL, url.QueryEscape(queueStr), firstID, len(uploadedIDs), url.QueryEscape(msg))
	if len(errorFiles) > 0 {
		redirectURL += "&warning=" + url.QueryEscape(i18n.T(lang, "compare.upload.warning_failed_prefix")+strings.Join(errorFiles, ", "))
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}
