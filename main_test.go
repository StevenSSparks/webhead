package main

import (
	"reflect"
	"testing"
)

func TestSplitImageArgs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantImage string
		wantRest  []string
	}{
		{"image then flag", []string{"./img", "--setup-hosts"}, "./img", []string{"--setup-hosts"}},
		{"flag then image", []string{"--setup-hosts", "./img"}, "", []string{"--setup-hosts", "./img"}},
		{"value flag then image", []string{"--http", ":8080", "./img"}, "", []string{"--http", ":8080", "./img"}},
		{"image only", []string{"./img"}, "./img", []string{}},
		{"no args", nil, "", nil},
		{"flags only", []string{"--extended"}, "", []string{"--extended"}},
		{"image then value flag", []string{"./img", "--http", ":8080"}, "./img", []string{"--http", ":8080"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			img, rest := splitImageArgs(c.args)
			if img != c.wantImage {
				t.Fatalf("image = %q, want %q", img, c.wantImage)
			}
			if !reflect.DeepEqual(rest, c.wantRest) {
				t.Fatalf("rest = %#v, want %#v", rest, c.wantRest)
			}
		})
	}
}
