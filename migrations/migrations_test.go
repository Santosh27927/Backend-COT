package migrations

import "testing"

func TestEmbeddedHistoryAndValidation(t *testing.T) {
	known, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(known) < 2 || known[0].Version != 1 || known[0].Name != "001_create_domain_schemas.sql" {
		t.Fatal("baseline migration missing")
	}
	for _, invalid := range [][]Migration{nil, {{Version: 0, Name: "x", SQL: "SELECT 1"}}, {{Version: 2, Name: "x", SQL: "SELECT 1"}}, {{Version: 1, Name: "", SQL: "SELECT 1"}}, {{Version: 1, Name: "x", SQL: ""}}, {{Version: 1, Name: "x", SQL: "SELECT 1"}, {Version: 1, Name: "y", SQL: "SELECT 2"}}} {
		if err := validate(invalid); err == nil {
			t.Fatal("invalid migration sequence accepted")
		}
	}
}
