package jsonvalue

import (
	"strings"
	"testing"
)

func TestDecodeKeepsLargeIntegers(t *testing.T) {
	v, err := Decode(strings.NewReader(`{"id":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := String(Map(v)["id"]); got != "9007199254740993" {
		t.Errorf("id = %s", got)
	}
}

func TestTruthy(t *testing.T) {
	v, _ := Decode(strings.NewReader(`[null,false,0,0.0,"",[],{},true,1,"x",[0],{"a":0}]`))
	values := Slice(v)
	for i, value := range values {
		if want := i >= 7; Truthy(value) != want {
			t.Errorf("Truthy(%v) = %v, want %v", value, !want, want)
		}
	}
}

func TestList(t *testing.T) {
	wrapped, _ := Decode(strings.NewReader(`{"items":[1,2]}`))
	bare, _ := Decode(strings.NewReader(`[1]`))
	other, _ := Decode(strings.NewReader(`{"other":[1]}`))

	if len(List(wrapped, "items")) != 2 || len(List(bare, "items")) != 1 || List(other, "items") != nil {
		t.Error("List did not read wrapped, bare and missing lists")
	}
}

func TestText(t *testing.T) {
	if got := Text(map[string]any{"q": "<a&b>"}); got != `{"q":"<a&b>"}` {
		t.Errorf("Text = %s, want no HTML escaping", got)
	}
}
