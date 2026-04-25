package lore

import "testing"

func TestStripDirectivesFromText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "trailing tag",
			in:   "Took an arrow to the knee. +injured",
			want: "Took an arrow to the knee.",
		},
		{
			name: "leading tag",
			in:   "+injured Took an arrow to the knee.",
			want: "Took an arrow to the knee.",
		},
		{
			name: "mid-sentence directives with semicolons and trailing prose",
			in:   "He's hurt. inventory += helm; health -= 3. We head home.",
			want: "He's hurt. We head home.",
		},
		{
			name: "directive-only description",
			in:   "+injured",
			want: "",
		},
		{
			name: "field assignment",
			in:   "Sleepy frontier town. population = 100",
			want: "Sleepy frontier town.",
		},
		{
			name: "leading directive then prose with period",
			in:   "+town. Always cloudy, eastern side of valley.",
			want: "Always cloudy, eastern side of valley.",
		},
		{
			name: "stacked leading directives",
			in:   "+town. +gothic. Scratch marks in the buildings.",
			want: "Scratch marks in the buildings.",
		},
		{
			name: "leading field assignment then prose",
			in:   "population = 100. Sleepy frontier town.",
			want: "Sleepy frontier town.",
		},
		{
			name: "mixed leading tag and field",
			in:   "+town. population = 133. Gothic and rundown.",
			want: "Gothic and rundown.",
		},
		{
			name: "mid-sentence field increment",
			in:   "Redbrands raid the square. population -= 50. Town panics.",
			want: "Redbrands raid the square. Town panics.",
		},
		{
			name: "trailing text-list field",
			in:   "Sildar hands us a longsword. inventory += longsword",
			want: "Sildar hands us a longsword.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events, _ := ParseDirectives(tc.in, "t.md", 1)
			got := stripDirectivesFromText(tc.in, events)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReplaceInlineAsidesWithName(t *testing.T) {
	world := NewWorld()
	world.Entities = []Entity{
		{Name: "Destroyed Town", Type: "landmark"},
		{Name: "Strahd", Type: "character"},
		{Name: "Strahd", Type: "horse"}, // ambiguous
	}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "unambiguous typed aside reduces to bare name",
			in:   "Madam Eva told us of a (Destroyed Town (landmark): NE of Lake Luna) worth a look.",
			want: "Madam Eva told us of a Destroyed Town worth a look.",
		},
		{
			name: "ambiguous aside keeps disambiguator",
			in:   "We met (Strahd (character): a vampire) at dusk.",
			want: "We met Strahd (character) at dusk.",
		},
		{
			name: "no aside leaves text untouched",
			in:   "Just plain prose here.",
			want: "Just plain prose here.",
		},
		{
			name: "non-header parens are left alone",
			in:   "She paused (briefly) before answering.",
			want: "She paused (briefly) before answering.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := replaceInlineAsidesWithName(tc.in, world)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
