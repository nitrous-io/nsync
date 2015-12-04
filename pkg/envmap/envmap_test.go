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

	contains := func(arr []string, item string) bool {
		for _, str := range arr {
			if str == item {
				return true
			}
		}
		return false
	}

	if eLen := len(e); eLen != 3 {
		t.Errorf("env returned should have 3 items, but got %d", eLen)
	}

	expectedItems := []string{"FOO=123", "BAR=bar=124", "BAZ="}

	for _, expectedItem := range expectedItems {
		if !contains(e, expectedItem) {
			t.Errorf(`Expect %v to have \"%s\"`, e, expectedItem)
		}
	}
}
