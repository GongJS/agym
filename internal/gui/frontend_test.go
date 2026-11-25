package gui

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestFrontendDOMSelectorsExistInIndexHTML(t *testing.T) {
	htmlBytes, err := os.ReadFile(filepath.Join("..", "..", "frontend", "index.html"))
	if err != nil {
		t.Fatalf("read frontend/index.html: %v", err)
	}
	jsBytes, err := os.ReadFile(filepath.Join("..", "..", "frontend", "main.js"))
	if err != nil {
		t.Fatalf("read frontend/main.js: %v", err)
	}

	idDeclRe := regexp.MustCompile(`\bid="([A-Za-z0-9_-]+)"`)
	htmlIDs := make(map[string]bool)
	for _, match := range idDeclRe.FindAllStringSubmatch(string(htmlBytes), -1) {
		htmlIDs[match[1]] = true
	}

	selectorRe := regexp.MustCompile(`"#([A-Za-z0-9_-]+)"`)
	matches := selectorRe.FindAllStringSubmatch(string(jsBytes), -1)
	if len(matches) == 0 {
		t.Fatal("expected at least one #id selector in frontend/main.js")
	}
	for _, match := range matches {
		id := match[1]
		if !htmlIDs[id] {
			t.Errorf("frontend/main.js references #%s, which does not exist in frontend/index.html", id)
		}
	}
}
