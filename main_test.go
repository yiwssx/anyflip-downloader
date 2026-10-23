package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreatePDFRejectsZeroChunkSize(t *testing.T) {
	if err := createPDF(filepath.Join(t.TempDir(), "book.pdf"), t.TempDir(), 0); err == nil {
		t.Fatal("expected zero chunk size to be rejected")
	}
}

func TestCreatePDFDoesNotTreatExistingPDFAsSuccess(t *testing.T) {
	output := filepath.Join(t.TempDir(), "original.pdf")
	const original = "user-owned data"
	if err := os.WriteFile(output, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := createPDF(output, t.TempDir(), 10); err == nil {
		t.Fatal("expected existing PDF to be rejected")
	}
	content, err := os.ReadFile(output)
	if err != nil || string(content) != original {
		t.Fatalf("original PDF was changed: data=%q err=%v", content, err)
	}
}
