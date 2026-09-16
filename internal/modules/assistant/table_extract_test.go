package assistant_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
)

func TestReadTabularContent_CSV(t *testing.T) {
	csvData := []byte("اسم الصنف,الكمية,السعر\nبنادول اكسترا,20,45\nكونجستال أقراص,10,32.5\n")
	table, ok := assistant.ReadTabularContent(csvData, "order.csv", 10)
	require.True(t, ok)
	require.Contains(t, table, "| اسم الصنف | الكمية | السعر |")
	require.Contains(t, table, "| بنادول اكسترا | 20 | 45 |")
	require.Contains(t, table, "| كونجستال أقراص | 10 | 32.5 |")
}

func TestReadTabularContent_XLSX(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellValue(sheet, "A1", "كود الصنف")
	_ = f.SetCellValue(sheet, "B1", "اسم الدواء")
	_ = f.SetCellValue(sheet, "C1", "الكمية")

	_ = f.SetCellValue(sheet, "A2", "1001")
	_ = f.SetCellValue(sheet, "B2", "أوجمنتين 1 جم")
	_ = f.SetCellValue(sheet, "C2", "5")

	var buf bytes.Buffer
	err := f.Write(&buf)
	require.NoError(t, err)

	table, ok := assistant.ReadTabularContent(buf.Bytes(), "catalog.xlsx", 50)
	require.True(t, ok)
	require.Contains(t, table, "| كود الصنف | اسم الدواء | الكمية |")
	require.Contains(t, table, "| 1001 | أوجمنتين 1 جم | 5 |")
}

func TestReadTabularContent_Truncation(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("الصنف,الكمية\n")
	for i := 1; i <= 20; i++ {
		sb.WriteString("دواء,1\n")
	}

	table, ok := assistant.ReadTabularContent([]byte(sb.String()), "long.csv", 5)
	require.True(t, ok)
	require.Contains(t, table, "تم عرض 5 صفاً من أصل 20 صفاً")
}

func TestReadTabularContent_NonSpreadsheet(t *testing.T) {
	table, ok := assistant.ReadTabularContent([]byte("random unformatted text without delimiters"), "doc.txt", 10)
	// A plain text without sheet structure returns false
	if ok {
		require.NotEmpty(t, table)
	}
	// Image bytes must return false
	imgTable, imgOk := assistant.ReadTabularContent([]byte("\xFF\xD8\xFF\xE0\x00\x10JFIF"), "pic.jpg", 10)
	require.False(t, imgOk)
	require.Empty(t, imgTable)
}
