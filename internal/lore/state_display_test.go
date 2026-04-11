package lore

import "testing"

func TestFormatStateEmpty(t *testing.T) {
	out := FormatStateBlock(nil, nil)
	if out != "" {
		t.Fatalf("expected empty, got %q", out)
	}
}

func TestFormatStateEmptyMaps(t *testing.T) {
	out := FormatStateBlock(map[string]bool{}, map[string]FieldValue{})
	if out != "" {
		t.Fatalf("expected empty, got %q", out)
	}
}

func TestFormatStateTagsOnly(t *testing.T) {
	tags := map[string]bool{"injured": true, "bleeding": true}
	out := FormatStateBlock(tags, nil)
	want := "+bleeding +injured"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestFormatStateNumericFieldInteger(t *testing.T) {
	fields := map[string]FieldValue{
		"population": {Kind: FieldNumeric, Number: 100},
	}
	out := FormatStateBlock(nil, fields)
	want := "population: 100"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestFormatStateNumericFieldDecimal(t *testing.T) {
	fields := map[string]FieldValue{
		"weight": {Kind: FieldNumeric, Number: 3.14},
	}
	out := FormatStateBlock(nil, fields)
	want := "weight: 3.14"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestFormatStateTextListField(t *testing.T) {
	fields := map[string]FieldValue{
		"inventory": {Kind: FieldText, Text: []string{"longsword", "chainmail"}},
	}
	out := FormatStateBlock(nil, fields)
	want := "inventory: chainmail, longsword"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestFormatStateTextSingleItemRendersAsScalar(t *testing.T) {
	fields := map[string]FieldValue{
		"status": {Kind: FieldText, Text: []string{"alive"}},
	}
	out := FormatStateBlock(nil, fields)
	if out != "status: alive" {
		t.Fatalf("got %q", out)
	}
}

func TestFormatStateQuotedItemsContainingSeparators(t *testing.T) {
	fields := map[string]FieldValue{
		"inventory": {Kind: FieldText, Text: []string{"potion, red", "sword"}},
	}
	out := FormatStateBlock(nil, fields)
	// Sorted alphabetically: "potion, red" < "sword"
	want := `inventory: "potion, red", sword`
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestFormatStateQuotesEqualsInItem(t *testing.T) {
	fields := map[string]FieldValue{
		"notes": {Kind: FieldText, Text: []string{"key = value"}},
	}
	out := FormatStateBlock(nil, fields)
	if out != `notes: "key = value"` {
		t.Fatalf("got %q", out)
	}
}

func TestFormatStateDoesNotQuoteHyphens(t *testing.T) {
	fields := map[string]FieldValue{
		"inventory": {Kind: FieldText, Text: []string{"two-handed-sword"}},
	}
	out := FormatStateBlock(nil, fields)
	if out != "inventory: two-handed-sword" {
		t.Fatalf("got %q", out)
	}
}

func TestFormatStateCombined(t *testing.T) {
	tags := map[string]bool{"injured": true}
	fields := map[string]FieldValue{
		"population": {Kind: FieldNumeric, Number: 42},
	}
	out := FormatStateBlock(tags, fields)
	want := "+injured\npopulation: 42"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestFormatStateFieldsSortedAlphabetically(t *testing.T) {
	fields := map[string]FieldValue{
		"zeta":  {Kind: FieldNumeric, Number: 1},
		"alpha": {Kind: FieldNumeric, Number: 2},
		"mu":    {Kind: FieldNumeric, Number: 3},
	}
	out := FormatStateBlock(nil, fields)
	want := "alpha: 2\nmu: 3\nzeta: 1"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}
