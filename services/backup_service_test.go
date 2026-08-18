package services

import "testing"

func TestCSVColumnList(t *testing.T) {
	columns, err := csvColumnList([]byte("id,name,subscription_ids\n1,demo,{}\n"))
	if err != nil {
		t.Fatalf("csvColumnList() error = %v", err)
	}
	if columns != `"id", "name", "subscription_ids"` {
		t.Fatalf("csvColumnList() = %q", columns)
	}
}

func TestCSVColumnListRejectsUnsafeOrDuplicateNames(t *testing.T) {
	tests := [][]byte{
		[]byte("id,name;drop table config_policies\n"),
		[]byte("id,id\n"),
		[]byte("\n"),
	}
	for _, input := range tests {
		if _, err := csvColumnList(input); err == nil {
			t.Fatalf("期望拒绝非法表头: %q", input)
		}
	}
}
