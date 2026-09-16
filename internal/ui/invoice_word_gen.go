package ui

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"io"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
)

// GenerateInvoiceWord builds an authentic Microsoft Word (.docx) document for an invoice.
func GenerateInvoiceWord(data *billing.PrintableInvoiceData, w io.Writer) error {
	zipBuf := new(bytes.Buffer)
	zw := zip.NewWriter(zipBuf)

	// 1. [Content_Types].xml
	ct, err := zw.Create("[Content_Types].xml")
	if err != nil {
		return err
	}
	_, _ = io.WriteString(ct, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`)

	// 2. _rels/.rels
	rels, err := zw.Create("_rels/.rels")
	if err != nil {
		return err
	}
	_, _ = io.WriteString(rels, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`)

	// 3. word/document.xml
	doc, err := zw.Create("word/document.xml")
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:pPr><w:jc w:val="center"/><w:bidi/></w:pPr>
      <w:r><w:rPr><w:b/><w:sz w:val="36"/><w:color w:val="0F172A"/></w:rPr>
        <w:t>منصة دوا 24 - فاتورة استلام وتوريد ضريبية</w:t>
      </w:r>
    </w:p>
    <w:p>
      <w:pPr><w:jc w:val="center"/><w:bidi/></w:pPr>
      <w:r><w:rPr><w:sz w:val="22"/><w:color w:val="475569"/></w:rPr>
        <w:t>` + html.EscapeString(fmt.Sprintf("رقم الفاتورة: %s | تاريخ الإصدار: %s | تاريخ الاستحقاق: %s",
		data.InvoiceNumber, data.IssueDate.Format("2006-01-02"), data.DueDate.Format("2006-01-02"))) + `</w:t>
      </w:r>
    </w:p>
    <w:p><w:pPr><w:bidi/></w:pPr></w:p>
    <w:p>
      <w:pPr><w:bidi/></w:pPr>
      <w:r><w:rPr><w:b/><w:sz w:val="24"/></w:rPr><w:t>بيانات المورد (البائع): </w:t></w:r>
      <w:r><w:rPr><w:sz w:val="24"/></w:rPr><w:t>` + html.EscapeString(func() string {
		if data.Vendor.DisplayName != "" {
			return data.Vendor.DisplayName
		}
		return data.Vendor.LegalName
	}()) + ` (الرقم الضريبي: ` + html.EscapeString(data.Vendor.TaxNumber) + `)</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:bidi/></w:pPr>
      <w:r><w:rPr><w:b/><w:sz w:val="24"/></w:rPr><w:t>بيانات العميل (المشتري): </w:t></w:r>
      <w:r><w:rPr><w:sz w:val="24"/></w:rPr><w:t>` + html.EscapeString(func() string {
		if data.Customer.DisplayName != "" {
			return data.Customer.DisplayName
		}
		return data.Customer.LegalName
	}()) + ` (الرقم الضريبي: ` + html.EscapeString(data.Customer.TaxNumber) + `)</w:t></w:r>
    </w:p>
    <w:p><w:pPr><w:bidi/></w:pPr></w:p>
    
    <!-- Table of Lines -->
    <w:tbl>
      <w:tblPr>
        <w:tblW w:w="5000" w:type="pct"/>
        <w:tblBorders>
          <w:top w:val="single" w:sz="4" w:space="0" w:color="CBD5E1"/>
          <w:bottom w:val="single" w:sz="4" w:space="0" w:color="CBD5E1"/>
          <w:insideH w:val="single" w:sz="4" w:space="0" w:color="E2E8F0"/>
          <w:insideV w:val="none"/>
        </w:tblBorders>
      </w:tblPr>
      <w:tr>
        <w:trPr><w:tblHeader/></w:trPr>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>#</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>الكود</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>اسم الصنف</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>رقم التشغيلة</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>تاريخ الصلاحية</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>الكمية</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="right"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>سعر الجمهور</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>الخصم</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="right"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>الإجمالي</w:t></w:r></w:p></w:tc>
      </w:tr>`)

	for idx, line := range data.Lines {
		discStr := formatDiscountPercent(line.DiscountPercent)
		buf.WriteString(fmt.Sprintf(`
      <w:tr>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="center"/></w:pPr><w:r><w:t>%d</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="center"/></w:pPr><w:r><w:t>%s</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/></w:pPr><w:r><w:t>%s</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="center"/></w:pPr><w:r><w:t>%s</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="center"/></w:pPr><w:r><w:t>%s</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="center"/></w:pPr><w:r><w:t>%d</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="right"/></w:pPr><w:r><w:t>%s ج.م</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="center"/></w:pPr><w:r><w:t>%s</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:pPr><w:bidi/><w:jc w:val="right"/></w:pPr><w:r><w:t>%s ج.م</w:t></w:r></w:p></w:tc>
      </w:tr>`, idx+1, html.EscapeString(line.SKU), html.EscapeString(line.ItemName), html.EscapeString(line.BatchNumber), html.EscapeString(line.ExpiryDate), line.Quantity, line.UnitPrice.String(), discStr, line.TotalPrice.String()))
	}

	buf.WriteString(`
    </w:tbl>
    <w:p><w:pPr><w:bidi/></w:pPr></w:p>
    <w:p>
      <w:pPr><w:jc w:val="left"/><w:bidi/></w:pPr>
      <w:r><w:rPr><w:b/><w:sz w:val="24"/></w:rPr><w:t>` + html.EscapeString("إجمالي الأصناف قبل الخصم: "+data.Subtotal.String()+" ج.م") + `</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:jc w:val="left"/><w:bidi/></w:pPr>
      <w:r><w:rPr><w:b/><w:sz w:val="24"/></w:rPr><w:t>` + html.EscapeString("إجمالي الخصم التجاري: -"+data.TotalDiscount.String()+" ج.م") + `</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:jc w:val="left"/><w:bidi/></w:pPr>
      <w:r><w:rPr><w:b/><w:sz w:val="24"/></w:rPr><w:t>` + html.EscapeString("إجمالي ضريبة القيمة المضافة: "+data.TotalTax.String()+" ج.م") + `</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:jc w:val="left"/><w:bidi/></w:pPr>
      <w:r><w:rPr><w:b/><w:sz w:val="28"/><w:color w:val="0284C7"/></w:rPr><w:t>` + html.EscapeString("الصافي الإجمالي المطلوب سداده: "+data.TotalAmount.String()+" ج.م") + `</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`)

	if _, err := doc.Write(buf.Bytes()); err != nil {
		return err
	}

	if err := zw.Close(); err != nil {
		return err
	}

	_, err = io.Copy(w, zipBuf)
	return err
}
