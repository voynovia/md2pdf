/*
 * Markdown to PDF Converter
 * Available at http://github.com/solworktech/md2pdf
 *
 * Copyright © 2018 Cecil New <cecil.new@gmail.com>.
 * Distributed under the MIT License.
 * See README.md for details.
 *
 * Dependencies
 * This package depends on two other packages:
 *
 * Go Markdown processor
 *   Available at https://github.com/gomarkdown/markdown
 *
 * fpdf - a PDF document generator with high level support for
 *   text, drawing and images.
 *   Available at https://codeberg.org/go-pdf/fpdf
 */

package mdtopdf

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomarkdown/markdown/parser"
)

func testit(inputf string, gohighlight bool, t *testing.T) {
	inputd := "./testdata/"
	input := path.Join(inputd, inputf)

	tracerfile := path.Join(inputd, strings.TrimSuffix(path.Base(input), path.Ext(input)))
	tracerfile += ".log"

	pdffile := path.Join(inputd, strings.TrimSuffix(path.Base(input), path.Ext(input)))
	pdffile += ".pdf"

	content, err := os.ReadFile(input)
	if err != nil {
		t.Errorf("%v:%v", input, err)
	}

	var opts []RenderOption
	if gohighlight {
		opts = []RenderOption{IsHorizontalRuleNewPage(true), SetSyntaxHighlightBaseDir("./highlight/syntax_files")}
	}
	params := PdfRendererParams{
		Orientation:     "",
		Papersz:         "",
		PdfFile:         pdffile,
		TracerFile:      tracerfile,
		Opts:            opts,
		Theme:           LIGHT,
		CustomThemeFile: "",
		FontFile:        "",
		FontName:        "",
	}
	r := NewPdfRenderer(params)
	r.Extensions = parser.NoIntraEmphasis | parser.Tables | parser.FencedCode | parser.Autolink | parser.Strikethrough | parser.SpaceHeadings | parser.HeadingIDs | parser.BackslashLineBreak | parser.DefinitionLists
	err = r.Process(content)
	if err != nil {
		t.Error(err)
	}
}

func TestTables(t *testing.T) {
	testit("Tables.text", false, t)
}

func TestMarkdownDocumenationBasic(t *testing.T) {
	testit("Markdown Documentation - Basics.text", false, t)
}

func TestMarkdownDocumenationSyntax(t *testing.T) {
	testit("syntax.md", false, t)
}

func TestMarkdownDocumenationColourSyntax(t *testing.T) {
	testit("syntax_highlighting.md", true, t)
}

func TestImage(t *testing.T) {
	testit("Image.text", false, t)
}

func TestAutoLinks(t *testing.T) {
	testit("Auto links.text", false, t)
}

func TestAmpersandEncoding(t *testing.T) {
	testit("Amps and angle encoding.text", false, t)
}

func TestInlineLinks(t *testing.T) {
	testit("Links, inline style.text", false, t)
}

func TestLists(t *testing.T) {
	testit("Ordered and unordered lists.md", false, t)
}

func TestStringEmph(t *testing.T) {
	testit("Strong and em together.text", false, t)
}

func TestTabs(t *testing.T) {
	testit("Tabs.text", false, t)
}

func TestBackslashEscapes(t *testing.T) {
	testit("Backslash escapes.text", false, t)
}

func TestBackquotes(t *testing.T) {
	testit("Blockquotes with code blocks.text", false, t)
}

func TestCodeBlocks(t *testing.T) {
	testit("Code Blocks.text", false, t)
}

func TestCodeSpans(t *testing.T) {
	testit("Code Spans.text", false, t)
}

func TestHardWrappedPara(t *testing.T) {
	testit("Hard-wrapped paragraphs with list-like lines no empty line before block.text", false, t)
}

func TestHardWrappedPara2(t *testing.T) {
	testit("Hard-wrapped paragraphs with list-like lines.text", false, t)
}

func TestHorizontalRules(t *testing.T) {
	testit("Horizontal rules.text", false, t)
}

func TestInlineHtmlSimple(t *testing.T) {
	testit("Inline HTML (Simple).text", false, t)
}

func TestInlineHtmlAdvanced(t *testing.T) {
	testit("Inline HTML (Advanced).text", false, t)
}

func TestInlineHtmlComments(t *testing.T) {
	testit("Inline HTML comments.text", false, t)
}

func TestTitleWithQuotes(t *testing.T) {
	testit("Literal quotes in titles.text", false, t)
}

func TestNestedBlockquotes(t *testing.T) {
	testit("Nested blockquotes.text", false, t)
}

func TestLinksReference(t *testing.T) {
	testit("Links, reference style.text", false, t)
}

func TestLinksShortcut(t *testing.T) {
	testit("Links, shortcut references.text", false, t)
}

func TestTidyness(t *testing.T) {
	testit("Tidyness.text", false, t)
}

func TestTableHeaderRepeat(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("| Header A | Header B | Header C |\n")
	sb.WriteString("|----------|----------|----------|\n")
	for i := range 80 {
		fmt.Fprintf(&sb, "| Row %d A | Row %d B | Row %d C |\n", i, i, i)
	}

	pdffile := path.Join("./testdata/", "TableHeaderRepeat.pdf")
	tracerfile := path.Join("./testdata/", "TableHeaderRepeat.log")

	r := NewPdfRenderer(PdfRendererParams{
		PdfFile: pdffile, TracerFile: tracerfile, Theme: LIGHT,
	})
	r.Extensions = parser.Tables
	if err := r.Process([]byte(sb.String())); err != nil {
		t.Fatal(err)
	}
	if r.Pdf.PageCount() < 2 {
		t.Fatalf("ожидалось минимум 2 страницы, получено %d", r.Pdf.PageCount())
	}
}

// TestTableEmptyHeader проверяет таблицу с пустой первой ячейкой заголовка и <br /> в body cells.
func TestTableEmptyHeader(t *testing.T) {
	md := `|                         | Deviation >10NM | Level change                   |
| ----------------------- | --------------- | ------------------------------ |
| EAST 000-179 magnetic | LEFT | DESCEND 300ft |
| WEST 180-359 magnetic | RIGHT | CLIMB 300ft |
`
	pdffile := path.Join("./testdata/", "TableEmptyHeader.pdf")
	tracerfile := path.Join("./testdata/", "TableEmptyHeader.log")

	r := NewPdfRenderer(PdfRendererParams{
		PdfFile: pdffile, TracerFile: tracerfile, Theme: LIGHT,
	})
	r.Extensions = parser.Tables
	if err := r.Process([]byte(md)); err != nil {
		t.Fatal(err)
	}
}

// TestTableEmptyHeaderBR проверяет таблицу с пустой первой ячейкой заголовка и <br /> в body cells.
func TestTableEmptyHeaderBR(t *testing.T) {
	md := "|                         | Deviation >10NM | Level change                   |\n" +
		"| ----------------------- | --------------- | ------------------------------ |\n" +
		"| EAST 000-179 magnetic | LEFT<br />RIGHT | DESCEND 300ft<br />CLIMB 300ft |\n" +
		"| WEST 180-359 magnetic | LEFT<br />RIGHT | CLIMB 300ft<br />DESCEND 300ft |\n"

	pdffile := path.Join("./testdata/", "TableEmptyHeaderBR.pdf")
	tracerfile := path.Join("./testdata/", "TableEmptyHeaderBR.log")

	r := NewPdfRenderer(PdfRendererParams{
		PdfFile: pdffile, TracerFile: tracerfile, Theme: LIGHT,
	})
	r.Extensions = parser.Tables
	if err := r.Process([]byte(md)); err != nil {
		t.Fatal(err)
	}
}

// TestMathInline проверяет, что $...$ не вызывает "Unknown node type: *ast.Math".
func TestMathInline(t *testing.T) {
	md := "Possession of a minimum of $100 + US $50 per person per day.\n"

	pdffile := path.Join("./testdata/", "MathInline.pdf")
	tracerfile := path.Join("./testdata/", "MathInline.log")

	r := NewPdfRenderer(PdfRendererParams{
		PdfFile: pdffile, TracerFile: tracerfile, Theme: LIGHT,
	})
	// CommonExtensions включает MathJax — используем их чтобы воспроизвести баг
	r.Extensions = parser.CommonExtensions
	if err := r.Process([]byte(md)); err != nil {
		t.Fatal(err)
	}
}

// minPNG генерирует минимальный PNG 2x2 пикселя.
func minPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 1, color.RGBA{B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImageLocalWithInputBaseURL(t *testing.T) {
	// структура:
	//   tmpDir/
	//     baseDir/
	//       assets/
	//         test.png   ← изображение
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "baseDir")
	assetsDir := filepath.Join(baseDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "test.png"), minPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}

	md := []byte("# Image test\n\n![test](assets/test.png)\n")

	pdfFile := filepath.Join(tmpDir, "out.pdf")
	r := NewPdfRenderer(PdfRendererParams{
		PdfFile: pdfFile, Theme: LIGHT,
	})
	r.InputBaseURL = baseDir
	if err := r.Process(md); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(pdfFile)
	if err != nil {
		t.Fatal(err)
	}
	// PDF с изображением содержит XObject (маркер встроенного изображения)
	if !bytes.Contains(data, []byte("/XObject")) {
		t.Error("PDF не содержит XObject — изображение не встроено")
	}
}
