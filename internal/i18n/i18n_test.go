package i18n

import "testing"

func TestAllKeysComplete(t *testing.T) {
	for key, m := range messages {
		for _, lang := range []Lang{EN, ZHCN, ZHTW} {
			if s, ok := m[lang]; !ok || s == "" {
				t.Errorf("key %q missing %s translation", key, lang)
			}
		}
		if s, ok := m[EN]; !ok || s == "" {
			t.Errorf("key %q missing en fallback", key)
		}
	}
}

func TestParse(t *testing.T) {
	for _, s := range []string{"en", "zh-CN", "zh-TW"} {
		if _, ok := Parse(s); !ok {
			t.Errorf("Parse(%q) should succeed", s)
		}
	}
	for _, s := range []string{"", "fr", "zh", "EN", "zh-cn"} {
		if _, ok := Parse(s); ok {
			t.Errorf("Parse(%q) should fail", s)
		}
	}
}

func TestT(t *testing.T) {
	if got := T(ZHCN, "err.appNotFound"); got == "err.appNotFound" || got == "" {
		t.Fatalf("zh-CN translation missing: %q", got)
	}
	if got := T(ZHTW, "err.appNotFound"); got == "err.appNotFound" || got == "" {
		t.Fatalf("zh-TW translation missing: %q", got)
	}
	if T(EN, "err.appNotFound") == "" {
		t.Fatal("en translation missing")
	}
	if got := T(ZHCN, "no.such.key"); got != "no.such.key" {
		t.Fatalf("unknown key should be returned verbatim, got %q", got)
	}
	if T("bad-lang", "err.appNotFound") != T(EN, "err.appNotFound") {
		t.Fatal("unknown lang should fall back to en")
	}
}
