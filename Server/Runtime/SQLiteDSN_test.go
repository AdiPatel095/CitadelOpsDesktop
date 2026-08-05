package Runtime

import (
	"net/url"
	"reflect"
	"testing"
)

func TestSQLiteFileDSN(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantHost string
		wantPath string
	}{
		{
			name:     "posix",
			filename: "/tmp/Citadel Ops/Operations.sqlite",
			wantPath: "/tmp/Citadel Ops/Operations.sqlite",
		},
		{
			name:     "windows drive",
			filename: `C:\Users\ajmas\OneDrive\Documents\CerberusX\Data\Runtime\Operations.sqlite`,
			wantPath: "/C:/Users/ajmas/OneDrive/Documents/CerberusX/Data/Runtime/Operations.sqlite",
		},
		{
			name:     "windows UNC",
			filename: `\\fileserver\profiles\Citadel Ops\Reports.sqlite`,
			wantHost: "fileserver",
			wantPath: "/profiles/Citadel Ops/Reports.sqlite",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn, err := SQLiteFileDSN(test.filename, "busy_timeout(5000)", "foreign_keys(1)")
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := url.Parse(dsn)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Scheme != "file" || parsed.Host != test.wantHost || parsed.Path != test.wantPath {
				t.Fatalf("SQLiteFileDSN(%q) = %q (scheme=%q host=%q path=%q)", test.filename, dsn, parsed.Scheme, parsed.Host, parsed.Path)
			}
			if got, want := parsed.Query()["_pragma"], []string{"busy_timeout(5000)", "foreign_keys(1)"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("pragmas = %#v, want %#v", got, want)
			}
		})
	}
}

func TestSQLiteFileDSNRejectsAmbiguousPaths(t *testing.T) {
	for _, filename := range []string{"", "Runtime/Operations.sqlite", `\\fileserver`} {
		if dsn, err := SQLiteFileDSN(filename); err == nil {
			t.Fatalf("SQLiteFileDSN(%q) = %q, want error", filename, dsn)
		}
	}
}
