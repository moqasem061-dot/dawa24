package ui

import "github.com/muhiya/dawa24-store/internal/shared/i18n"

// The downloadable import template.
//
// One definition backs both the .xlsx and the .csv download, because two copies
// drift: the CSV template shipped ten columns and the Excel template shipped a
// different ten, and neither carried a SKU column — so an admin who filled in
// the supplied template had no identifier for the importer to match on, and
// every re-upload of a corrected file created a second copy of every product.
//
// A round-trip test parses this exact content back through the importer, so the
// file the admin downloads is guaranteed to be one the importer reads correctly.

// importSampleHeaders are written in the vocabulary the column mapper scores
// highest, so a template filled in as-is maps with full confidence.
var importSampleHeaders = []string{
	i18n.T("ar", "ingest.col.name_ar"),
	i18n.T("ar", "ingest.col.name_en"),
	i18n.T("ar", "ingest.col.sku"),
	i18n.T("ar", "ingest.col.barcode"),
	i18n.T("ar", "ingest.col.scientific_name"),
	i18n.T("ar", "ingest.col.active_ingredient"),
	i18n.T("ar", "ingest.col.dosage_form"),
	i18n.T("ar", "ingest.col.concentration"),
	i18n.T("ar", "ingest.col.unit"),
	i18n.T("ar", "ingest.col.manufacturer"),
	i18n.T("ar", "ingest.col.public_price"),
	i18n.TDefault("w4_ui.s_10_10"),
	i18n.TDefault("w4_ui.s_11_11"),
}

// importSampleRows show one filled example per common dosage form with public price.
var importSampleRows = [][]string{
	{
		i18n.TDefault("w4_ui.s_12_12"), "Congestal Tablets", "CONG-TAB-650", "6221234567890",
		"Paracetamol + Pseudoephedrine", "Paracetamol 500mg", i18n.TDefault("w4_ui.s_13_13"), "650mg", i18n.TDefault("w4_ui.s_14_14"),
		"Eva Pharma", "30.00",
		i18n.TDefault("w4_ui.s_15_15"), "For cold and flu relief",
	},
	{
		i18n.TDefault("w4_ui.s_16_16"), "Panadol Extra", "PAN-EXT-500", "6229876543210",
		"Paracetamol + Caffeine", "Paracetamol 500mg + Caffeine 65mg", i18n.TDefault("w4_ui.s_13_13"), "500mg", i18n.TDefault("w4_ui.s_14_14"),
		"GSK", "40.00",
		i18n.TDefault("w4_ui.s_17_17"), "Pain reliever and fever reducer",
	},
	{
		i18n.TDefault("w4_ui.1_18"), "Augmentin 1g Tablets", "AUG-1G", "6223334445556",
		"Amoxicillin + Clavulanic Acid", "Amoxicillin 875mg + Clavulanate 125mg", i18n.TDefault("w4_ui.s_13_13"), "1g", i18n.TDefault("w4_ui.s_14_14"),
		"GlaxoSmithKline", "95.00",
		i18n.TDefault("w4_ui.s_19_19"), "Broad spectrum antibiotic",
	},
	{
		i18n.TDefault("w4_ui.s_20_20"), "Antinal Capsules", "ANTIN-CAP-200", "6224445556667",
		"Nifuroxazide", "Nifuroxazide 200mg", i18n.TDefault("w4_ui.s_21_21"), "200mg", i18n.TDefault("w4_ui.s_22_22"),
		"Amoun Pharmaceutical", "30.00",
		i18n.TDefault("w4_ui.s_23_23"), "Intestinal antiseptic",
	},
	{
		i18n.TDefault("w4_ui.s_24_24"), "Catafast Sachets", "CATA-SACH-50", "6227778889990",
		"Diclofenac Potassium", "Diclofenac Potassium 50mg", i18n.TDefault("w4_ui.s_25_25"), "50mg", i18n.TDefault("w4_ui.s_26_26"),
		"Novartis", "65.00",
		i18n.TDefault("w4_ui.s_27_27"), "Fast acting pain relief",
	},
}
