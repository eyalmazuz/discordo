package config

import (
	"testing"

	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

func TestStyleWrapperUnmarshalTOML(t *testing.T) {
	t.Run("invalid type", func(t *testing.T) {
		var sw StyleWrapper
		if err := sw.UnmarshalTOML("invalid"); err != errInvalidType {
			t.Fatalf("expected errInvalidType, got %v", err)
		}
	})

	t.Run("parses colors attributes and underline variants", func(t *testing.T) {
		cases := map[string]tcell.UnderlineStyle{
			"":        tcell.UnderlineStyleNone,
			"solid":   tcell.UnderlineStyleSolid,
			"double":  tcell.UnderlineStyleDouble,
			"curly":   tcell.UnderlineStyleCurly,
			"dotted":  tcell.UnderlineStyleDotted,
			"dashed":  tcell.UnderlineStyleDashed,
		}

		for name := range cases {
			var sw StyleWrapper
			err := sw.UnmarshalTOML(map[string]any{
				"foreground":      "red",
				"background":      "blue",
				"attributes":      []any{"bold", "italic", "dim", "strikethrough", "underline", "blink", "reverse"},
				"underline":       name,
				"underline_color": "green",
			})
			if err != nil {
				t.Fatalf("underline=%q: unexpected error: %v", name, err)
			}

			if fg := sw.GetForeground(); fg != tcell.GetColor("red") {
				t.Fatalf("underline=%q: foreground = %v, want %v", name, fg, tcell.GetColor("red"))
			}
			if bg := sw.GetBackground(); bg != tcell.GetColor("blue") {
				t.Fatalf("underline=%q: background = %v, want %v", name, bg, tcell.GetColor("blue"))
			}
			attrs := sw.GetAttributes()
			for _, attr := range []tcell.AttrMask{tcell.AttrBold, tcell.AttrItalic, tcell.AttrDim, tcell.AttrStrikeThrough, tcell.AttrBlink, tcell.AttrReverse} {
				if attrs&attr == 0 {
					t.Fatalf("underline=%q: expected attributes %v to include %v", name, attrs, attr)
				}
			}
			if name != "" && sw.GetUnderlineStyle() == tcell.UnderlineStyleNone {
				t.Fatalf("underline=%q: expected underline style to be set", name)
			}
		}
	})

	t.Run("single string attribute is parsed", func(t *testing.T) {
		var sw StyleWrapper
		if err := sw.UnmarshalTOML(map[string]any{"attributes": "bold"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if attrs := sw.GetAttributes(); attrs&tcell.AttrBold == 0 {
			t.Fatalf("expected bold attribute, got %v", attrs)
		}
	})
}

func TestWrapperUnmarshalHelpers(t *testing.T) {
	t.Run("alignment invalid type", func(t *testing.T) {
		var aw AlignmentWrapper
		if err := aw.UnmarshalTOML(123); err != errInvalidType {
			t.Fatalf("expected errInvalidType, got %v", err)
		}
	})

	t.Run("alignment values", func(t *testing.T) {
		cases := map[string]tview.Alignment{
			"left":   tview.AlignmentLeft,
			"center": tview.AlignmentCenter,
			"right":  tview.AlignmentRight,
		}
		for input, want := range cases {
			var aw AlignmentWrapper
			if err := aw.UnmarshalTOML(input); err != nil {
				t.Fatalf("input=%q: unexpected error: %v", input, err)
			}
			if aw.Alignment != want {
				t.Fatalf("input=%q: alignment = %v, want %v", input, aw.Alignment, want)
			}
		}
	})

	t.Run("border set invalid type", func(t *testing.T) {
		var bw BorderSetWrapper
		if err := bw.UnmarshalTOML(123); err != errInvalidType {
			t.Fatalf("expected errInvalidType, got %v", err)
		}
	})

	t.Run("border set values", func(t *testing.T) {
		for _, input := range []string{"hidden", "plain", "round", "thick", "double"} {
			var bw BorderSetWrapper
			if err := bw.UnmarshalTOML(input); err != nil {
				t.Fatalf("input=%q: unexpected error: %v", input, err)
			}
		}
	})

	t.Run("glyph and scrollbar invalid type", func(t *testing.T) {
		var gw GlyphSetWrapper
		if err := gw.UnmarshalTOML(123); err != errInvalidType {
			t.Fatalf("expected errInvalidType, got %v", err)
		}
		var vw ScrollBarVisibilityWrapper
		if err := vw.UnmarshalTOML(123); err != errInvalidType {
			t.Fatalf("expected errInvalidType, got %v", err)
		}
	})

	t.Run("glyph values", func(t *testing.T) {
		for _, input := range []string{"minimal", "box_drawing", "boxdrawing", "box", "unicode"} {
			var gw GlyphSetWrapper
			if err := gw.UnmarshalTOML(input); err != nil {
				t.Fatalf("input=%q: unexpected error: %v", input, err)
			}
		}
	})

	t.Run("scrollbar visibility values", func(t *testing.T) {
		for _, input := range []string{"automatic", "auto", "always", "never", "hidden", "off"} {
			var vw ScrollBarVisibilityWrapper
			if err := vw.UnmarshalTOML(input); err != nil {
				t.Fatalf("input=%q: unexpected error: %v", input, err)
			}
		}
	})
}
