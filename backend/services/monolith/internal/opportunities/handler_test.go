package opportunities

import (
	"archive/zip"
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestValidateOpportunity(t *testing.T) {
	future := time.Now().UTC().AddDate(0, 0, 2).Format("2006-01-02")
	base := opportunityRequest{
		Type: "siwes", Title: "Training placement", Location: "Lagos",
		WorkMode: "On-site", Summary: "Supervised training",
		Description: "Placement details", ApplicationDeadline: &future,
	}
	if err := validateOpportunity(base, false); err != nil {
		t.Fatalf("valid draft rejected: %v", err)
	}
	if err := validateOpportunity(base, true); err != nil {
		t.Fatalf("valid publish rejected: %v", err)
	}
	base.ApplicationDeadline = nil
	if err := validateOpportunity(base, true); err == nil {
		t.Fatal("publish without a deadline accepted")
	}
}

func TestApplicationTransitions(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{"submitted", "under_review", true},
		{"submitted", "rejected", true},
		{"under_review", "shortlisted", true},
		{"under_review", "accepted", true},
		{"shortlisted", "rejected", true},
		{"accepted", "under_review", false},
		{"rejected", "accepted", false},
		{"submitted", "accepted", false},
	}
	for _, test := range tests {
		if got := validApplicationTransition(test.from, test.to); got != test.want {
			t.Errorf("validApplicationTransition(%q, %q) = %t, want %t",
				test.from, test.to, got, test.want)
		}
	}
}

func TestParseApplicationFormAllowsOptionalMessage(t *testing.T) {
	form := &multipart.Form{
		Value: map[string][]string{
			"full_name": {"Ada Example"},
			"email": {"ada@example.edu"},
			"phone": {"+2348000000000"},
			"institution": {"Example University"},
			"course": {"Engineering"},
			"matric_number": {"ENG/2024/001"},
		},
		File: map[string][]*multipart.FileHeader{
			"document": []*multipart.FileHeader{&multipart.FileHeader{}},
		},
	}
	input, err := parseApplicationForm(form)
	if err != nil {
		t.Fatalf("valid application without message rejected: %v", err)
	}
	if input.Message != "" {
		t.Fatalf("optional message = %q, want empty", input.Message)
	}
	form.Value["unknown"] = []string{"unexpected"}
	if _, err := parseApplicationForm(form); err == nil {
		t.Fatal("unknown multipart field accepted")
	}
}

func TestValidateDocumentChecksExtensionAndContent(t *testing.T) {
	pdf := []byte("%PDF-1.7\nexample")
	if contentType, ok := validateDocument("letter.pdf", pdf); !ok || contentType != "application/pdf" {
		t.Fatalf("valid PDF = (%q, %t)", contentType, ok)
	}
	if _, ok := validateDocument("letter.doc", pdf); ok {
		t.Fatal("PDF content with a DOC extension accepted")
	}
}

func TestValidateDocumentChecksDOCXContainer(t *testing.T) {
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for _, name := range []string{"[Content_Types].xml", "word/document.xml"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("<xml/>")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	contentType, ok := validateDocument("letter.docx", data.Bytes())
	if !ok || contentType != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("valid DOCX = (%q, %t)", contentType, ok)
	}
	if _, ok := validateDocument("letter.docx", []byte("PK\x03\x04invalid")); ok {
		t.Fatal("invalid DOCX container accepted")
	}
}

func TestParseListQuery(t *testing.T) {
	query, err := parseListQuery(mapValues("type=siwes&limit=25&offset=2"), true)
	if err != nil {
		t.Fatal(err)
	}
	if query.Type != "siwes" || query.Limit != 25 || query.Offset != 2 {
		t.Fatalf("unexpected query: %+v", query)
	}
	if _, err := parseListQuery(mapValues("type=invalid"), false); err == nil {
		t.Fatal("invalid opportunity type accepted")
	}
	if _, err := parseListQuery(mapValues("limit=101"), true); err == nil {
		t.Fatal("overlarge page size accepted")
	}
}

func TestAnonymousWriteRequiresSameOrigin(t *testing.T) {
	handler := NewHandler(nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/opportunities", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("anonymous write without Origin status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("request ID response header missing")
	}
}

func mapValues(encoded string) url.Values {
	values, err := url.ParseQuery(encoded)
	if err != nil {
		panic(err)
	}
	return values
}
