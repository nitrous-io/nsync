package syncignore

import "testing"

func TestParseLine(t *testing.T) {
	testParseLine := func(input string, expectedResult []string) {
		result := ParseLine(input)

		if len(result) != len(expectedResult) {
			t.Errorf("expected to have %d items, but got %d items\nexpected to have %+v, but got %+v",
				len(expectedResult), len(result), expectedResult, result)
			return
		}

		for i := 0; i < len(result); i++ {
			if result[i] != expectedResult[i] {
				t.Errorf("input: %s, expected %s, but got %s", input, expectedResult[i], result[i])
				return
			}
		}
	}

	testParseLine("config.ru", []string{"Name config.ru"})
	testParseLine("#hello", []string{})
	testParseLine("   ", []string{})
	testParseLine("/path/to/ignore", []string{"Path path/to/ignore"})
	testParseLine("/path/to/**", []string{"Regex path/to/.*"})
	testParseLine("path/to/**", []string{"Regex .*/path/to/.*", "Regex path/to/.*"})
	testParseLine("path/to/", []string{"Regex .*/path/to/", "Regex path/to/.*", "Path path/to"})
	testParseLine("/path/to/", []string{"Regex .*/path/to/", "Regex path/to/.*", "Path path/to"})
	testParseLine("/pa.th/to/", []string{`Regex .*/pa\.th/to/`, `Regex pa\.th/to/.*`, "Path pa.th/to"})
	testParseLine("pa.th/to/", []string{`Regex .*/pa\.th/to/`, `Regex pa\.th/to/.*`, "Path pa.th/to"})
	testParseLine(`(p)+a^$.?|t[h]/\t{o}/`, []string{`Regex .*/\(p\)\+a\^\$\.\?\|t\[h\]/\\t\{o\}/`, `Regex \(p\)\+a\^\$\.\?\|t\[h\]/\\t\{o\}/.*`, `Path (p)+a^$.?|t[h]/\t{o}`})
}
