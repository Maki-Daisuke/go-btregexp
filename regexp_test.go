package btregexp

import (
	"testing"
)

func TestBasicMatching(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		want    bool
	}{
		{"a", "a", true},
		{"a", "b", false},
		{"abc", "abc", true},
		{"abc", "abcd", true},
		{"abc", "ab", false},
		{"a.c", "abc", true},
		{"a.c", "axc", true},
		{"a.c", "ac", false},
		{"a.*c", "ac", true},
		{"a.*c", "abc", true},
		{"a.*c", "abcdefgc", true},
		{"a.*c", "abcdefg", false},
		{"a.+c", "ac", false},
		{"a.+c", "abc", true},
		{"a.+c", "abcdefgc", true},
		{"^abc", "abc", true},
		{"^abc", "xabc", false},
		{"abc$", "abc", true},
		{"abc$", "abcx", false},
		{"^abc$", "abc", true},
		{"^abc$", "abcx", false},
		{"^abc$", "xabc", false},
		{"a?b", "b", true},
		{"a?b", "ab", true},
		{"a?b", "aab", true},
		{"a+b", "b", false},
		{"a+b", "ab", true},
		{"a+b", "aab", true},
	}

	for _, tt := range tests {
		re, err := Compile(tt.pattern)
		if err != nil {
			t.Errorf("Compile(%q) failed: %v", tt.pattern, err)
			continue
		}

		got := re.MatchString(tt.input)
		if got != tt.want {
			t.Errorf("Compile(%q).MatchString(%q) = %v, want %v", tt.pattern, tt.input, got, tt.want)
		}
	}
}

func TestGroupCapture(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		matches []string
	}{
		{"(a)", "a", []string{"a", "a"}},
		{"(a)b", "ab", []string{"ab", "a"}},
		{"a(b)c", "abc", []string{"abc", "b"}},
		{"(a)(b)(c)", "abc", []string{"abc", "a", "b", "c"}},
		{"(a(b)c)", "abc", []string{"abc", "abc", "b"}},
		{"a(.)c", "abc", []string{"abc", "b"}},
		{"a(.*)c", "abbc", []string{"abbc", "bb"}},
		{"a(.+)c", "abbc", []string{"abbc", "bb"}},
	}

	for _, tt := range tests {
		re, err := Compile(tt.pattern)
		if err != nil {
			t.Errorf("Compile(%q) failed: %v", tt.pattern, err)
			continue
		}

		matches := re.FindStringSubmatch(tt.input)
		if matches == nil {
			t.Errorf("Compile(%q).FindStringSubmatch(%q) returned nil, want %v", tt.pattern, tt.input, tt.matches)
			continue
		}

		if len(matches) != len(tt.matches) {
			t.Errorf("Compile(%q).FindStringSubmatch(%q) returned %d groups, want %d groups: %v",
				tt.pattern, tt.input, len(matches), len(tt.matches), matches)
			continue
		}

		for i, expected := range tt.matches {
			if matches[i] != expected {
				t.Errorf("Compile(%q).FindStringSubmatch(%q)[%d] = %q, want %q",
					tt.pattern, tt.input, i, matches[i], expected)
			}
		}
	}
}

func TestFind(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		want    string
	}{
		{"a", "xay", "a"},
		{"abc", "xxabcyy", "abc"},
		{"a.c", "xxabcyy", "abc"},
		{"a.*c", "xxabcyy", "abc"},
		{"a.*c", "xxabyczy", "abyc"},
		{"a.+c", "xxabcyy", "abc"},
	}

	for _, tt := range tests {
		re, err := Compile(tt.pattern)
		if err != nil {
			t.Errorf("Compile(%q) failed: %v", tt.pattern, err)
			continue
		}

		got := re.FindString(tt.input)
		if got != tt.want {
			t.Errorf("Compile(%q).FindString(%q) = %q, want %q", tt.pattern, tt.input, got, tt.want)
		}
	}
}

func TestFindAll(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		n       int
		want    []string
	}{
		{"a", "banana", -1, []string{"a", "a", "a"}},
		{"a", "banana", 2, []string{"a", "a"}},
		{"an", "banana", -1, []string{"an", "an"}},
		{"an+", "banana", -1, []string{"an", "an"}},
		{"a.+", "abacad", -1, []string{"abacad"}},
		{"a.", "abacad", -1, []string{"ab", "ac", "ad"}},
	}

	for _, tt := range tests {
		re, err := Compile(tt.pattern)
		if err != nil {
			t.Errorf("Compile(%q) failed: %v", tt.pattern, err)
			continue
		}

		got := re.FindAllString(tt.input, tt.n)
		if len(got) != len(tt.want) {
			t.Errorf("Compile(%q).FindAllString(%q, %d) = %q, want %q",
				tt.pattern, tt.input, tt.n, got, tt.want)
			continue
		}

		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("Compile(%q).FindAllString(%q, %d)[%d] = %q, want %q",
					tt.pattern, tt.input, tt.n, i, got[i], tt.want[i])
			}
		}
	}
}

func TestReplaceAll(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		repl    string
		want    string
	}{
		{"a", "banana", "x", "bxnxnx"},
		{"a", "banana", "$0", "banana"},
		{"(an)", "banana", "[$1]", "b[an][an]a"},
		{"a(.)", "abacad", "x$1", "xbxcxd"},
	}

	for _, tt := range tests {
		re, err := Compile(tt.pattern)
		if err != nil {
			t.Errorf("Compile(%q) failed: %v", tt.pattern, err)
			continue
		}

		got := re.ReplaceAllString(tt.input, tt.repl)
		if got != tt.want {
			t.Errorf("Compile(%q).ReplaceAllString(%q, %q) = %q, want %q",
				tt.pattern, tt.input, tt.repl, got, tt.want)
		}
	}
}

func TestSplit(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		n       int
		want    []string
	}{
		{"a", "banana", -1, []string{"b", "n", "n", ""}},
		{"a", "banana", 2, []string{"b", "nana"}},
		{"an", "banana", -1, []string{"b", "", "a"}},
		{",", "a,b,c", -1, []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		re, err := Compile(tt.pattern)
		if err != nil {
			t.Errorf("Compile(%q) failed: %v", tt.pattern, err)
			continue
		}

		got := re.Split(tt.input, tt.n)
		if len(got) != len(tt.want) {
			t.Errorf("Compile(%q).Split(%q, %d) = %q, want %q",
				tt.pattern, tt.input, tt.n, got, tt.want)
			continue
		}

		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("Compile(%q).Split(%q, %d)[%d] = %q, want %q",
					tt.pattern, tt.input, tt.n, i, got[i], tt.want[i])
			}
		}
	}
}

func TestFlags(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		want    bool
	}{
		{"a", "A", false},
		{"(?i)a", "A", true},
		{"a", "A", false},
		{"(?i:a)", "A", true},
		{"(?i:a)b", "Ab", true},
		{"(?i:a)b", "AB", false},
		{"(?i:a)(?-i:b)", "Ab", true},
		{"(?i:a)(?-i:b)", "AB", false},
		{"(?m)^a", "a", true},
		{"(?m)^a", "\na", true},
		{"(?s)a.b", "a\nb", true},
		{"a.b", "a\nb", false},
	}

	for _, tt := range tests {
		re, err := Compile(tt.pattern)
		if err != nil {
			t.Errorf("Compile(%q) failed: %v", tt.pattern, err)
			continue
		}

		got := re.MatchString(tt.input)
		if got != tt.want {
			t.Errorf("Compile(%q).MatchString(%q) = %v, want %v", tt.pattern, tt.input, got, tt.want)
		}
	}
}
