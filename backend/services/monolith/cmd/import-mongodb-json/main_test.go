package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNullableTimeFieldParsesRelaxedAndCanonicalExtendedJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want time.Time
	}{
		{
			name: "relaxed extended JSON",
			raw:  `{"created_at":{"$date":"2026-11-01T07:17:59.406Z"}}`,
			want: time.Date(2026, 11, 1, 7, 17, 59, 406000000, time.UTC),
		},
		{
			name: "canonical extended JSON",
			raw:  `{"created_at":{"$date":{"$numberLong":"1793517479406"}}}`,
			want: time.Date(2026, 11, 1, 7, 17, 59, 406000000, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var doc document
			if err := json.Unmarshal([]byte(test.raw), &doc); err != nil {
				t.Fatal(err)
			}
			got, err := optionalTimeField(doc, "created_at")
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || !got.Equal(test.want) {
				t.Fatalf("optionalTimeField() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestOptionalTimeFieldRejectsMalformedDate(t *testing.T) {
	var doc document
	if err := json.Unmarshal([]byte(`{"created_at":{"$date":"not-a-date"}}`), &doc); err != nil {
		t.Fatal(err)
	}
	if _, err := optionalTimeField(doc, "created_at"); err == nil {
		t.Fatal("optionalTimeField accepted a malformed date")
	}
}

func TestStringFieldAcceptsLegacyFieldNamesCaseInsensitively(t *testing.T) {
	var doc document
	if err := json.Unmarshal([]byte(`{"accountid":"00000000-0000-0000-0000-000000000001"}`), &doc); err != nil {
		t.Fatal(err)
	}
	if got := stringField(doc, "account_id", "accountid"); got != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("stringField() = %q", got)
	}
}

func TestImportFileRowsRequiresDeclaredExports(t *testing.T) {
	directory := t.TempDir()
	count, err := importFileRows(t.Context(), nil, directory, importFile{name: "missing", required: false})
	if err != nil || count != 0 {
		t.Fatalf("optional missing export returned count=%d, err=%v", count, err)
	}

	_, err = importFileRows(t.Context(), nil, directory, importFile{name: "missing", required: true})
	if err == nil {
		t.Fatal("required missing export was accepted")
	}
}

func TestImportFileRowsParsesOneRecordPerLine(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "records.json")
	if err := os.WriteFile(path, []byte("{\"id\":\"first\"}\n{\"id\":\"second\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	count, err := importFileRows(t.Context(), nil, directory, importFile{
		name: "records",
		importFn: func(_ context.Context, _ *sql.Tx, doc document) error {
			if stringField(doc, "id") == "" {
				t.Fatal("record has no id")
			}
			return nil
		},
	})
	if err != nil || count != 2 {
		t.Fatalf("importFileRows() = count %d, err %v; want count 2", count, err)
	}
}
