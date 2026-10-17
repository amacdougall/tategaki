// Package text rasterises UI text (library, menus, later the lookup popup)
// with a bundled Japanese TrueType font. Book pages are pre-rendered by the
// converter; this is only for the reader's own screens.
package text

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Fonts holds the parsed UI typefaces and caches faces by size.
type Fonts struct {
	regular, bold *opentype.Font
	faces         map[faceKey]font.Face
}

type faceKey struct {
	px   float64
	bold bool
}

func parse(path string) (*opentype.Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Load parses the regular and bold UI fonts.
func Load(regularPath, boldPath string) (*Fonts, error) {
	r, err := parse(regularPath)
	if err != nil {
		return nil, err
	}
	b, err := parse(boldPath)
	if err != nil {
		return nil, err
	}
	return &Fonts{regular: r, bold: b, faces: map[faceKey]font.Face{}}, nil
}

// Face returns a face at px pixels per em.
func (f *Fonts) Face(px float64, bold bool) font.Face {
	k := faceKey{px, bold}
	if face, ok := f.faces[k]; ok {
		return face
	}
	src := f.regular
	if bold {
		src = f.bold
	}
	face, err := opentype.NewFace(src, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err) // only fails for invalid options
	}
	f.faces[k] = face
	return face
}

// Width is the advance width of s in pixels.
func Width(face font.Face, s string) int {
	return font.MeasureString(face, s).Ceil()
}

// LineHeight is the face's recommended line spacing in pixels.
func LineHeight(face font.Face) int {
	return face.Metrics().Height.Ceil()
}

// Draw renders s with its baseline at (x, y).
func Draw(dst draw.Image, face font.Face, x, y int, s string, c color.Color) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

// No line may start with these (a light version of kinsoku shori).
const noLineStart = "、。，．・：；？！ー」』）〕］｝〉》】ぁぃぅぇぉっゃゅょゎァィゥェォッャュョヮヵヶ…‥"

// Wrap breaks s into lines at most width pixels wide. CJK text breaks
// between any characters; Latin text breaks at spaces where possible. If
// maxLines > 0, the last kept line is ellipsised when text remains.
func Wrap(face font.Face, s string, width, maxLines int) []string {
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		lines = append(lines, wrapPara(face, para, width)...)
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = Ellipsize(face, lines[maxLines-1]+"…", width)
	}
	return lines
}

func wrapPara(face font.Face, s string, width int) []string {
	runes := []rune(s)
	if len(runes) == 0 {
		return []string{""}
	}
	var lines []string
	start := 0
	for start < len(runes) {
		end := start
		for end < len(runes) && Width(face, string(runes[start:end+1])) <= width {
			end++
		}
		if end == start {
			end = start + 1 // a single glyph wider than the line
		}
		if end < len(runes) {
			// prefer breaking after a space in Latin text
			for i := end; i > start+1; i-- {
				if runes[i-1] == ' ' {
					end = i
					break
				}
				if runes[i-1] > 0x2E80 { // CJK: any position is fine
					break
				}
			}
			// kinsoku: pull a forbidden line-start character back onto this line
			for end < len(runes) && end-start > 1 && strings.ContainsRune(noLineStart, runes[end]) {
				end--
			}
		}
		lines = append(lines, strings.TrimRight(string(runes[start:end]), " "))
		start = end
		for start < len(runes) && runes[start] == ' ' {
			start++
		}
	}
	return lines
}

// Ellipsize shortens s with a trailing "…" until it fits width.
func Ellipsize(face font.Face, s string, width int) string {
	if Width(face, s) <= width {
		return s
	}
	r := []rune(strings.TrimSuffix(s, "…"))
	for len(r) > 0 && Width(face, string(r)+"…") > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
