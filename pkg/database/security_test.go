package database

import "testing"

func TestFormatSQLInterpolatesOnlyPortablePlaceholders(t *testing.T) {
	for _, testCase := range []struct {
		driver string
		want   string
	}{
		{driver: "sqlite", want: "SELECT id FROM markers WHERE trackID = ?"},
		{driver: "chai", want: "SELECT id FROM markers WHERE trackID = ?"},
		{driver: "duckdb", want: "SELECT id FROM markers WHERE trackID = ?"},
		{driver: "pgx", want: "SELECT id FROM markers WHERE trackID = $1"},
	} {
		got := formatSQL("SELECT id FROM markers WHERE trackID = %s", placeholder(testCase.driver, 1))
		if got != testCase.want {
			t.Fatalf("%s query = %q, want %q", testCase.driver, got, testCase.want)
		}
	}
}
