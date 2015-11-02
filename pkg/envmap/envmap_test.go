package envmap

import "testing"

func TestEnvToMap(t *testing.T) {
	m := EnvToMap([]string{
		"FOO=123",
		"BAR=bar=124",
		"BAZ=",
		"",
	})

	if mLen := len(m); mLen != 3 {
		t.Errorf("map should have 3 items, but got %d", mLen)
	}

	if foo := m["FOO"]; foo != "123" {
		t.Errorf("FOO should equal \"123\", but got %s", foo)
	}

	if bar := m["BAR"]; bar != "bar=124" {
		t.Errorf("BAR should equal \"bar=124\", but got %s", bar)
	}

	if baz := m["BAZ"]; baz != "" {
		t.Errorf("BAZ should be an empty string, but got %s", baz)
	}
}

func TestMapToEnv(t *testing.T) {
	e := MapToEnv(map[string]string{
		"FOO": "123",
		"BAR": "bar=124",
		"BAZ": "",
	})

	if eLen := len(e); eLen != 3 {
		t.Errorf("env returned should have 3 items, but got %d", eLen)
	}

	if foo := e[0]; foo != "FOO=123" {
		t.Errorf("First item should equal \"FOO=123\", but got %s", foo)
	}

	if bar := e[1]; bar != "BAR=bar=124" {
		t.Errorf("Second item should equal \"BAR=bar=124\", but got %s", bar)
	}

	if baz := e[2]; baz != "BAZ=" {
		t.Errorf("Third item should equal \"BAZ=\", but got %s", baz)
	}
}
