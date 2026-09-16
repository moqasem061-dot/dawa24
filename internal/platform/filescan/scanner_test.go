package filescan_test

import (
	"encoding/base64"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/filescan"
)

func decodeB64(s string) []byte {
	b, _ := base64.StdEncoding.DecodeString(s)
	return b
}

func TestScan_ValidFiles(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		filename string
		mime     string
	}{
		{
			name:     "valid pdf",
			data:     []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<<>>\n%%EOF"),
			filename: "license.pdf",
			mime:     "application/pdf",
		},
		{
			name:     "valid png",
			data:     append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, []byte("IHDR....IDAT....IEND")...),
			filename: "avatar.png",
			mime:     "image/png",
		},
		{
			name:     "valid jpeg",
			data:     append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}, []byte("image_data")...),
			filename: "product.jpg",
			mime:     "image/jpeg",
		},
		{
			name:     "valid csv",
			data:     []byte("item_name,price,quantity\nPanadol,25.50,10\nAmoxil,40.00,5\n"),
			filename: "items.csv",
			mime:     "text/csv",
		},
		{
			name:     "valid svg",
			data:     []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><circle cx="50" cy="50" r="40" fill="yellow" /></svg>`),
			filename: "logo.svg",
			mime:     "image/svg+xml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := filescan.Scan(tc.data, tc.filename, tc.mime)
			if !res.Passed {
				t.Fatalf("expected file to pass scan, got failure: %s (verdict: %s)", res.Reason, res.Verdict)
			}
		})
	}
}

func TestScan_MaliciousAndTamperedFiles(t *testing.T) {
	tests := []struct {
		name            string
		data            []byte
		filename        string
		mime            string
		expectedVerdict filescan.Verdict
	}{
		{
			name:            "empty file",
			data:            []byte{},
			filename:        "empty.pdf",
			mime:            "application/pdf",
			expectedVerdict: filescan.VerdictCorrupted,
		},
		{
			name:            "PE windows executable disguised as pdf",
			data:            []byte{'M', 'Z', 0x90, 0x00, 0x03, 0x00, 0x00, 0x00},
			filename:        "trojan.pdf",
			mime:            "application/pdf",
			expectedVerdict: filescan.VerdictExecutableBinary,
		},
		{
			name:            "ELF linux executable disguised as png",
			data:            []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00},
			filename:        "exploit.png",
			mime:            "image/png",
			expectedVerdict: filescan.VerdictExecutableBinary,
		},
		{
			name:            "image containing embedded PHP tag",
			data:            append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("<" + "?php phpinfo(); ?" + ">")...),
			filename:        "backdoor.jpg",
			mime:            "image/jpeg",
			expectedVerdict: filescan.VerdictDangerousContent,
		},
		{
			name:            "PDF with active JavaScript execution action",
			data:            []byte("%PDF-1.4\n<< /Type /Action /S /Java" + "Script /JS (alert(1);) >>"),
			filename:        "malicious_form.pdf",
			mime:            "application/pdf",
			expectedVerdict: filescan.VerdictEmbeddedScript,
		},
		{
			name:            "SVG with embedded script tag",
			data:            []byte(`<svg xmlns="http://www.w3.org/2000/svg"><scr` + `ipt>1</scr` + `ipt></svg>`),
			filename:        "xss.svg",
			mime:            "image/svg+xml",
			expectedVerdict: filescan.VerdictEmbeddedScript,
		},
		{
			name:            "SVG with inline onload event",
			data:            []byte(`<svg on` + `load="test()"></svg>`),
			filename:        "xss_onload.svg",
			mime:            "image/svg+xml",
			expectedVerdict: filescan.VerdictEmbeddedScript,
		},
		{
			name:            "PDF with header mismatch",
			data:            []byte("Random text content without PDF header"),
			filename:        "fake.pdf",
			mime:            "application/pdf",
			expectedVerdict: filescan.VerdictHeaderMismatch,
		},
		{
			name:            "CSV with embedded null byte",
			data:            []byte("header1,header2\x00payload,bad"),
			filename:        "binary.csv",
			mime:            "text/csv",
			expectedVerdict: filescan.VerdictDangerousContent,
		},
		{
			name:            "CSV with spreadsheet formula injection",
			data:            []byte("name,quantity\n=c" + "md|'/C calc'!A0,10"),
			filename:        "exploit.csv",
			mime:            "text/csv",
			expectedVerdict: filescan.VerdictDangerousContent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := filescan.Scan(tc.data, tc.filename, tc.mime)
			if res.Passed {
				t.Fatalf("expected file to fail scan, but passed: %v", res)
			}
			if res.Verdict != tc.expectedVerdict {
				t.Fatalf("expected verdict %s, got %s (reason: %s)", tc.expectedVerdict, res.Verdict, res.Reason)
			}
		})
	}
}
