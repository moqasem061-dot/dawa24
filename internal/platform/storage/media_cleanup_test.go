package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteUploadedMedia(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("UPLOAD_DIR", tempDir)

	adsDir := filepath.Join(tempDir, "ads")
	if err := os.MkdirAll(adsDir, 0755); err != nil {
		t.Fatalf("mkdir ads dir: %v", err)
	}

	mainFile := filepath.Join(adsDir, "ads_test123.jpg")
	thumbFile := filepath.Join(adsDir, "ads_test123_thumb.jpg")
	cardFile := filepath.Join(adsDir, "ads_test123_card.jpg")
	fullFile := filepath.Join(adsDir, "ads_test123_full.png")
	unrelatedFile := filepath.Join(adsDir, "ads_other456.jpg")

	for _, f := range []string{mainFile, thumbFile, cardFile, fullFile, unrelatedFile} {
		if err := os.WriteFile(f, []byte("test content"), 0644); err != nil {
			t.Fatalf("write test file %s: %v", f, err)
		}
	}

	// Test deleting via URL with /uploads/ prefix
	err := DeleteUploadedMedia(context.Background(), "/uploads/ads/ads_test123.jpg", nil)
	if err != nil {
		t.Fatalf("DeleteUploadedMedia failed: %v", err)
	}

	for _, f := range []string{mainFile, thumbFile, cardFile, fullFile} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("expected file %s to be deleted, but it still exists", f)
		}
	}

	if _, err := os.Stat(unrelatedFile); err != nil {
		t.Errorf("unrelated file should not have been deleted: %v", err)
	}
}

func TestDeleteUploadedMedia_PathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("UPLOAD_DIR", tempDir)

	secretFile := filepath.Join(tempDir, "secret.txt")
	_ = os.WriteFile(secretFile, []byte("secret"), 0644)

	err := DeleteUploadedMedia(context.Background(), "/uploads/ads/../../secret.txt", nil)
	if err != nil {
		t.Fatalf("DeleteUploadedMedia returned error on traversal attempt: %v", err)
	}

	if _, err := os.Stat(secretFile); err != nil {
		t.Errorf("secret file outside path should not have been deleted: %v", err)
	}
}
