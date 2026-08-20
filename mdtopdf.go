/*
 * Markdown to PDF Converter
 * Available at http://github.com/solworktech/md2pdf
 *
 * Copyright © Cecil New <cecil.new@gmail.com>, Jesse Portnoy <jesse@packman.io>.
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

// Package mdtopdf converts markdown to PDF.
package mdtopdf

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"unicode/utf8"

	"strings"

	"codeberg.org/go-pdf/fpdf"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
)

// Color is a RGB set of ints; for a nice picker
// see https://www.w3schools.com/colors/colors_picker.asp
type Color struct {
	Red, Green, Blue int
}

// Styler is the struct to capture the styling features for text
// Size and Spacing are specified in points.
// The sum of Size and Spacing is used as line height value
// in the fpdf API
type Styler struct {
	Font      string
	Style     string
	Size      float64
	Spacing   float64
	TextColor Color
	FillColor Color
}

// RenderOption allows to define functions to configure the renderer
type RenderOption func(r *PdfRenderer)

// Theme [light|dark]
type Theme int

const (
	// DARK theme const
	DARK Theme = 1
	// LIGHT theme const
	LIGHT Theme = 2
	// CUSTOM theme const
	CUSTOM Theme = 3
)

// PdfRenderer is the struct to manage conversion of a markdown object
// to PDF format.
type PdfRenderer struct {
	// Pdf can be used to access the underlying created fpdf object
	// prior to processing the markdown source
	Pdf                *fpdf.Fpdf
	orientation, units string
	papersize, fontdir string

	// trace/log file if present
	pdfFile, tracerFile string
	w                   *bufio.Writer

	// default margins for safe keeping
	mleft, mtop, mright, mbottom float64

	// normal text
	Normal            Styler
	em                float64
	unicodeTranslator func(string) string

	// link text
	Link Styler

	// backticked text
	Backtick Styler

	// blockquote text
	Blockquote  Styler
	IndentValue float64

	// Headings
	H1 Styler
	H2 Styler
	H3 Styler
	H4 Styler
	H5 Styler
	H6 Styler

	// Table styling
	THeader Styler
	TBody   Styler

	cs states

	// code styling
	Code Styler

	// update styling
	NeedCodeStyleUpdate       bool
	NeedBlockquoteStyleUpdate bool
	HorizontalRuleNewPage     bool
	SyntaxHighlightBaseDir    string
	InputBaseURL              string
	Theme                     Theme
	BackgroundColor           Color
	documentMatter            ast.DocumentMatters // keep track of front/main/back matter.
	Extensions                parser.Extensions
	ColumnWidths              map[ast.Node][]float64

	// LandscapeWideTables разворачивает страницу в альбомную ориентацию под
	// таблицу, натуральная ширина которой заметно шире портретной полосы.
	LandscapeWideTables bool
	// таблицы, отобранные setColumnWidths под альбомную страницу
	landscapeTables map[ast.Node]bool
	// признак того, что текущая страница развёрнута под таблицу
	inLandscape bool
	// альбомная страница дозаполняется обычным потоком после таблицы
	landscapeTail bool
	// блоки, которые стоит забрать на альбомную страницу перед таблицей
	landscapePrelude map[ast.Node]bool
	// рендерится блок из такого преддверия
	inPrelude bool
	// разрыв страницы запрещён: идёт замыкающая линия таблицы нулевой высоты
	suppressBreak bool

	tocLinks map[string]*int
}

// TOCEntry represents a table of contents entry
type TOCEntry struct {
	Level int
	Title string
	ID    string
}

// TOCVisitor implements ast.NodeVisitor to collect headers
type TOCVisitor struct {
	Entries []TOCEntry
}

// Visit implements the ast.NodeVisitor interface
func (v *TOCVisitor) Visit(node ast.Node, entering bool) ast.WalkStatus {
	if !entering {
		return ast.GoToNext
	}

	// Check if the node is a heading
	if heading, ok := node.(*ast.Heading); ok {
		// Extract the text content from the heading
		title := ExtractTextFromNode(heading)
		if title != "" {
			// Create a simple ID from the title (lowercase, replace spaces with hyphens)
			id := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(title), " ", "-"))
			// Remove special characters for cleaner IDs
			id = strings.ReplaceAll(id, ".", "")
			id = strings.ReplaceAll(id, ",", "")
			id = strings.ReplaceAll(id, "!", "")
			id = strings.ReplaceAll(id, "?", "")

			entry := TOCEntry{
				Level: heading.Level,
				Title: title,
				ID:    id,
			}
			v.Entries = append(v.Entries, entry)
		}
	}

	return ast.GoToNext
}

// ExtractTextFromNode recursively extracts text content from AST nodes
func ExtractTextFromNode(node ast.Node) string {
	var text strings.Builder

	ast.WalkFunc(node, func(node ast.Node, entering bool) ast.WalkStatus {
		if entering {
			switch n := node.(type) {
			case *ast.Text:
				text.Write(n.Literal)
			case *ast.Code:
				text.Write(n.Literal)
			}
		}
		return ast.GoToNext
	})

	return strings.ReplaceAll(text.String(), "\t", "    ")
}

// GetTOCEntries returns TOC entries
func GetTOCEntries(content []byte) ([]TOCEntry, error) {

	// Create parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs
	p := parser.NewWithExtensions(extensions)

	// Parse the markdown content
	doc := markdown.Parse(content, p)

	// Create visitor to collect TOC entries
	visitor := &TOCVisitor{}

	// Walk the AST and collect headers
	ast.Walk(doc, visitor)

	return visitor.Entries, nil
}

// SetTOCLinks these will be used in `nodeProcessing.go:processText()` if the header is encoutered
// as we need to call `r.Pdf.SetLink()` if that's the case
func (r *PdfRenderer) SetTOCLinks(tocHeaders map[string]*int) {
	r.tocLinks = tocHeaders
}

// SetLightTheme sets theme to 'light'
func (r *PdfRenderer) SetLightTheme() {
	r.BackgroundColor = Colorlookup("white")
	r.SetPageBackground("", r.BackgroundColor)
	// Normal Text
	r.Normal = Styler{Font: "Arial", Style: "", Size: 12, Spacing: 2,
		TextColor: Colorlookup("black"), FillColor: Colorlookup("white")}

	// Link text
	r.Link = Styler{Font: "Arial", Style: "b", Size: 12, Spacing: 2,
		TextColor: Colorlookup("cornflowerblue")}

	// Backticked text
	r.Backtick = Styler{Font: "Times", Style: "", Size: 12, Spacing: 2,
		TextColor: Color{37, 27, 14}, FillColor: Color{200, 200, 200}}

	// Quoted Text

	r.Blockquote = Styler{Font: "Times", Style: "", Size: 12, Spacing: 2,
		TextColor: Color{37, 27, 14}, FillColor: Color{200, 200, 200}}

	// Code text
	r.Code = Styler{Font: "Times", Style: "", Size: 12, Spacing: 2,
		TextColor: Color{37, 27, 14}, FillColor: Color{200, 200, 200}}

	// Headings
	r.H1 = Styler{Font: "Arial", Style: "b", Size: 24, Spacing: 5,
		TextColor: Colorlookup("black"), FillColor: Colorlookup("white")}
	r.H2 = Styler{Font: "Arial", Style: "b", Size: 22, Spacing: 5,
		TextColor: Colorlookup("black"), FillColor: Colorlookup("white")}
	r.H3 = Styler{Font: "Arial", Style: "b", Size: 20, Spacing: 5,
		TextColor: Colorlookup("black"), FillColor: Colorlookup("white")}
	r.H4 = Styler{Font: "Arial", Style: "b", Size: 18, Spacing: 5,
		TextColor: Colorlookup("black"), FillColor: Colorlookup("white")}
	r.H5 = Styler{Font: "Arial", Style: "b", Size: 16, Spacing: 5,
		TextColor: Colorlookup("black"), FillColor: Colorlookup("white")}
	r.H6 = Styler{Font: "Arial", Style: "b", Size: 14, Spacing: 5,
		TextColor: Colorlookup("black"), FillColor: Colorlookup("white")}

	r.Blockquote = Styler{Font: "Arial", Style: "i", Size: 12, Spacing: 2,
		TextColor: Colorlookup("black"), FillColor: Colorlookup("white")}

	// Table Header Text
	r.THeader = Styler{Font: "Arial", Style: "b", Size: 12, Spacing: 2,
		TextColor: Colorlookup("black"), FillColor: Color{180, 180, 180}}

	// Table Body Text
	r.TBody = Styler{Font: "Arial", Style: "", Size: 12, Spacing: 2,
		TextColor: Colorlookup("black"), FillColor: Color{240, 240, 240}}
}

// SetDarkTheme sets theme to 'dark'
func (r *PdfRenderer) SetDarkTheme() {
	r.BackgroundColor = Colorlookup("black")
	r.SetPageBackground("", r.BackgroundColor)
	// Normal Text
	r.Normal = Styler{Font: "Arial", Style: "", Size: 12, Spacing: 2,
		FillColor: Colorlookup("black"), TextColor: Colorlookup("white")}

	// Quoted Text
	r.Blockquote = Styler{Font: "Arial", Style: "", Size: 12, Spacing: 2,
		FillColor: Colorlookup("black"), TextColor: Colorlookup("white")}

	// Link text
	r.Link = Styler{Font: "Arial", Style: "b", Size: 12, Spacing: 2,
		TextColor: Colorlookup("cornflowerblue")}

	// Backticked text
	r.Backtick = Styler{Font: "Times", Style: "", Size: 12, Spacing: 2,
		TextColor: Colorlookup("lightgrey"), FillColor: Color{32, 35, 37}}

	// Code text
	r.Code = Styler{Font: "Times", Style: "", Size: 12, Spacing: 2,
		TextColor: Colorlookup("lightgrey"), FillColor: Color{32, 35, 37}}

	// Headings
	r.H1 = Styler{Font: "Arial", Style: "b", Size: 24, Spacing: 5,
		FillColor: Colorlookup("black"), TextColor: Colorlookup("darkgray")}
	r.H2 = Styler{Font: "Arial", Style: "b", Size: 22, Spacing: 5,
		FillColor: Colorlookup("black"), TextColor: Colorlookup("darkgray")}
	r.H3 = Styler{Font: "Arial", Style: "b", Size: 20, Spacing: 5,
		FillColor: Colorlookup("black"), TextColor: Colorlookup("darkgray")}
	r.H4 = Styler{Font: "Arial", Style: "b", Size: 18, Spacing: 5,
		FillColor: Colorlookup("black"), TextColor: Colorlookup("darkgray")}
	r.H5 = Styler{Font: "Arial", Style: "b", Size: 16, Spacing: 5,
		FillColor: Colorlookup("black"), TextColor: Colorlookup("darkgray")}
	r.H6 = Styler{Font: "Arial", Style: "b", Size: 14, Spacing: 5,
		FillColor: Colorlookup("black"), TextColor: Colorlookup("darkgray")}

	r.Blockquote = Styler{Font: "Arial", Style: "i", Size: 12, Spacing: 2,
		FillColor: Colorlookup("black"), TextColor: Colorlookup("darkgray")}

	// Table Header Text
	r.THeader = Styler{Font: "Arial", Style: "b", Size: 12, Spacing: 2,
		TextColor: Colorlookup("darkgray"), FillColor: Color{27, 27, 27}}

	// Table Body Text
	r.TBody = Styler{Font: "Arial", Style: "", Size: 12, Spacing: 2,
		FillColor: Color{200, 200, 200}, TextColor: Color{128, 128, 128}}

}

// SetCustomTheme sets a custom theme based on JSON config
func (r *PdfRenderer) SetCustomTheme(themeJSONFile string) {

	config, err := os.ReadFile(themeJSONFile)
	if err != nil {
		log.Fatal(err)
	}
	// Fill the instance from the JSON file content
	err = json.Unmarshal(config, &r)
	// Check if is there any error while filling the instance
	if err != nil {
		log.Fatal("Error parsing ", themeJSONFile, ":\n", err)
	}
}

// PdfRendererParams struct to hold params passed to NewPdfRenderer
type PdfRendererParams struct {
	Orientation, Papersz, PdfFile, TracerFile, FontFile, FontName string
	Opts                                                          []RenderOption
	Theme                                                         Theme
	CustomThemeFile                                               string
}

// NewPdfRenderer creates and configures an PdfRenderer object,
// which satisfies the Renderer interface.
func NewPdfRenderer(params PdfRendererParams) *PdfRenderer {

	r := new(PdfRenderer)

	// set filenames
	r.pdfFile = params.PdfFile
	r.tracerFile = params.TracerFile

	// Global things
	r.orientation = "portrait"
	if params.Orientation != "" {
		r.orientation = params.Orientation
	}

	r.units = "pt"
	r.papersize = "Letter"
	if params.Papersz != "" {
		r.papersize = params.Papersz
	}

	r.fontdir = "."

	r.Theme = params.Theme

	r.Pdf = fpdf.New(r.orientation, r.units, r.papersize, r.fontdir)

	r.Pdf.SetHeaderFunc(func() {
		r.SetPageBackground("", r.BackgroundColor)
	})

	switch r.Theme {
	case DARK:
		r.SetDarkTheme()
	case LIGHT:
		r.SetLightTheme()
	case CUSTOM:
		if params.CustomThemeFile != "" {
			r.SetCustomTheme(params.CustomThemeFile)
		}
	}
	r.Pdf.AddPage()
	// set default font
	r.setStyler(r.Normal)
	r.mleft, r.mtop, r.mright, r.mbottom = r.Pdf.GetMargins()
	// Перехватчик разрывов нужен любому документу: он гасит разрыв под
	// замыкающую линию таблицы, а не только возвращает в портрет после
	// альбомной. Вне своих случаев отдаёт ровно то, что вернул бы fpdf.
	r.Pdf.SetAcceptPageBreakFunc(r.acceptPageBreak)
	r.em = r.Pdf.GetStringWidth("m")
	r.IndentValue = 3 * r.em

	r.cs = states{stack: make([]*containerState, 0)}
	initcurrent := &containerState{
		listkind:  notlist,
		textStyle: r.Normal, leftMargin: r.mleft}
	r.cs.push(initcurrent)

	for _, o := range params.Opts {
		o(r)
	}

	// Синхронизация начального контейнера с финальным r.Normal.
	// Opts callbacks могут изменить r.Normal (через json.Unmarshal),
	// начальный textStyle должен отражать финальное значение,
	// иначе standalone-параграфы используют устаревший стиль.
	r.cs.stack[0].textStyle = r.Normal

	return r
}

// NewPdfRendererWithDefaultStyler creates and configures an PdfRenderer object,
// which satisfies the Renderer interface.
// update default styler for normal
func NewPdfRendererWithDefaultStyler(orient, papersz, pdfFile, tracerFile string, defaultStyler Styler, opts []RenderOption, theme Theme) *PdfRenderer {
	opts = append(opts, func(r *PdfRenderer) {
		r.Normal = defaultStyler
	})
	params := PdfRendererParams{
		Orientation: orient,
		Papersz:     papersz,
		PdfFile:     pdfFile,
		TracerFile:  tracerFile,
		Opts:        opts,
		Theme:       theme,
	}

	return NewPdfRenderer(params)
}

// Process takes the markdown content, parses it to generate the PDF
func (r *PdfRenderer) Process(content []byte) error {
	// try to open tracer
	var f *os.File
	var err error
	if r.tracerFile != "" {
		f, err = os.Create(r.tracerFile)
		if err != nil {
			return fmt.Errorf("os.Create() on tracefile error:%v", err)
		}
		defer f.Close()
		r.w = bufio.NewWriter(f)
		defer r.w.Flush()
	}

	err = r.Run(content)
	if err != nil {
		return fmt.Errorf("error on %v:%v", r.pdfFile, err)
	}

	err = r.Pdf.OutputFileAndClose(r.pdfFile)
	if err != nil {
		return fmt.Errorf("error on %v:%v", r.pdfFile, err)
	}

	return nil
}

// preprocessTables убирает ведущий whitespace у строк, являющихся частью
// таблицы (начинаются с '|' после trim). Это позволяет парсеру распознавать
// таблицы с отступом внутри list items, где gomarkdown их иначе игнорирует.
// Строки внутри fenced code blocks не затрагиваются.
func preprocessTables(content []byte) []byte {
	lines := bytes.Split(content, []byte("\n"))
	inFence := false
	changed := false
	for i, line := range lines {
		trimmed := bytes.TrimLeft(line, " \t")
		if bytes.HasPrefix(trimmed, []byte("```")) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		// Строка с indent + pipe — потенциальная строка таблицы.
		// Не трогаем строки без отступа (уже корректны для парсера).
		if len(trimmed) < len(line) && len(trimmed) > 0 && trimmed[0] == '|' {
			lines[i] = trimmed
			changed = true
		}
	}
	if !changed {
		return content
	}
	return bytes.Join(lines, []byte("\n"))
}

// Run takes the markdown content, parses it but don't generate the PDF. you can access the PDF with youRenderer.Pdf
func (r *PdfRenderer) Run(content []byte) error {
	// Preprocess content by changing all CRLF to LF
	s := content
	s = markdown.NormalizeNewlines(s)

	if r.unicodeTranslator != nil {
		s = []byte(r.unicodeTranslator(string(s)))
	}

	// Убрать отступы у строк таблиц внутри list items,
	// чтобы парсер мог распознать их как *ast.Table.
	s = preprocessTables(s)

	p := parser.NewWithExtensions(r.Extensions)
	doc := markdown.Parse(s, p)

	setColumnWidths(doc, r)
	markLandscapePreludes(doc, r)
	_ = markdown.Render(doc, r)
	// футер последней страницы рисуется при закрытии документа — поле полосы
	// набора к этому моменту обязано быть обычным
	r.Pdf.SetRightMargin(r.mright)

	return nil
}

// Parses all tables and sets the column width to the longest string in that column
func setColumnWidths(doc ast.Node, r *PdfRenderer) {
	columnWidths := map[ast.Node][]float64{}
	landscapeTables := map[ast.Node]bool{}
	intable := false
	inheader := true
	cellnum := 0
	lengths := []float64{}
	minWidths := []float64{}
	textlength := float64(0)
	ast.WalkFunc(doc, func(node ast.Node, entering bool) ast.WalkStatus {
		switch n := node.(type) {
		case *ast.Table:
			if entering {
				intable = true
			} else {
				intable = false
				// масштабировать ширины столбцов до ширины страницы
				pageW, pageH := r.Pdf.GetPageSize()
				_, _, rightM, _ := r.Pdf.GetMargins()
				usableW := pageW - r.mleft - rightM
				// Минимум каждого столбца = ширина самого длинного слова
				// плюс поля ячейки. Это одновременно нижняя граница ширины
				// столбца: колонка уже минимума рвёт слово по буквам, что
				// портит даже таблицу, которая целиком помещается в полосу.
				cellPad := 2 * r.Pdf.GetCellMargin()
				totalW, totalMinW := 0.0, 0.0
				for i := range lengths {
					if i < len(minWidths) {
						minWidths[i] += cellPad
						if lengths[i] < minWidths[i] {
							lengths[i] = minWidths[i]
						}
						totalMinW += minWidths[i]
					}
					totalW += lengths[i]
				}
				// широкая таблица получает альбомную страницу и
				// масштабируется уже под её полосу
				if landscapeW := pageH - r.mleft - rightM; r.wantsLandscape(totalW, totalMinW, usableW, landscapeW) {
					landscapeTables[node] = true
					usableW = landscapeW
				}
				if totalW > usableW {
					// Оставшееся пространство распределяется пропорционально.
					if totalMinW >= usableW {
						// Даже минимумы не помещаются — равное распределение
						equalW := usableW / float64(len(lengths))
						for i := range lengths {
							lengths[i] = equalW
						}
					} else {
						remainingW := usableW - totalMinW
						totalExtra := 0.0
						for i, w := range lengths {
							if extra := w - minWidths[i]; extra > 0 {
								totalExtra += extra
							}
						}
						for i := range lengths {
							extra := lengths[i] - minWidths[i]
							if extra > 0 && totalExtra > 0 {
								lengths[i] = minWidths[i] + remainingW*(extra/totalExtra)
							} else {
								lengths[i] = minWidths[i]
							}
						}
					}
				}
				columnWidths[node] = lengths
			}

		case *ast.TableHeader:
			inheader = entering
			if entering {
				lengths = []float64{}
				minWidths = []float64{}
			}
		case *ast.TableRow:
			if entering {
				cellnum = 0
			}
		case *ast.TableCell:
			if entering {
				if inheader {
					lengths = append(lengths, 0)
					minWidths = append(minWidths, 0)
				}
			} else {
				textlength += textlength * 0.2

				currentMax := lengths[cellnum]
				if textlength > currentMax {
					lengths[cellnum] = textlength
				}
				textlength = 0
				cellnum++
			}
		case *ast.Text:
			if entering && intable {
				text := string(n.Literal)
				l := r.Pdf.GetStringWidth(text)
				textlength += l
				// Отслеживать ширину самого длинного слова для минимума столбца
				for _, word := range strings.Fields(text) {
					ww := r.Pdf.GetStringWidth(word)
					ww += ww * 0.2 // тот же 20% запас, что и для натуральных ширин
					if cellnum < len(minWidths) && ww > minWidths[cellnum] {
						minWidths[cellnum] = ww
					}
				}
			}
		}
		return ast.GoToNext
	})
	r.ColumnWidths = columnWidths
	r.landscapeTables = landscapeTables
}

// landscapePreludeBudget — сколько знаков текста перед широкой таблицей стоит
// забрать на её альбомную страницу. Больше — и текст занял бы страницу целиком,
// не оставив места самой таблице.
const landscapePreludeBudget = 1500

// markLandscapePreludes отмечает блоки, которые уедут на альбомную страницу
// вместе с идущей за ними широкой таблицей. Ориентация страницы выбирается в
// момент её начала, поэтому текст перед таблицей иначе остаётся на портретной
// странице, обрывает её на середине и оставляет пустоту до самого низа: сама
// таблица портретную страницу дописать уже не может.
//
// Назад от таблицы отбираются подряд идущие блоки, пока их суммарный объём
// держится в бюджете; таблица, горизонтальная линия или превышение бюджета
// прекращают отбор.
func markLandscapePreludes(doc ast.Node, r *PdfRenderer) {
	prelude := map[ast.Node]bool{}
	children := doc.GetChildren()
	for i, node := range children {
		table, ok := node.(*ast.Table)
		if !ok || !r.landscapeTables[table] {
			continue
		}
		budget := landscapePreludeBudget
		for j := i - 1; j >= 0; j-- {
			switch children[j].(type) {
			case *ast.Table, *ast.HorizontalRule:
				j = -1
				continue
			}
			size := blockTextLen(children[j])
			if size > budget {
				break
			}
			budget -= size
			prelude[children[j]] = true
		}
	}
	r.landscapePrelude = prelude
}

// blockTextLen — грубая оценка объёма блока: знаки его текста плюс надбавка на
// сам блок (отбивка, маркеры, заголовок).
func blockTextLen(node ast.Node) int {
	n := 40
	ast.WalkFunc(node, func(nd ast.Node, entering bool) ast.WalkStatus {
		if entering {
			if t, ok := nd.(*ast.Text); ok {
				n += utf8.RuneCount(t.Literal)
			}
		}
		return ast.GoToNext
	})
	return n
}

// wantsLandscape сообщает, нужна ли таблице альбомная страница. Разворот
// стоит документу разрыва страницы и пустого места до и после таблицы,
// поэтому его заслуживает лишь та таблица, которой портретная полоса
// безнадёжна: натуральная ширина naturalW превышает даже альбомную полосу
// (перенос слов неизбежен в любой ориентации, и альбом честно забирает всю
// добавочную ширину) либо портретная полоса уже́ суммы самых длинных слов
// minW, то есть столбцы пришлось бы рвать по буквам. Умеренный избыток между
// портретной и альбомной полосой таблица переживает переносом слов на месте —
// это дешевле пустых страниц.
func (r *PdfRenderer) wantsLandscape(naturalW, minW, portraitW, landscapeW float64) bool {
	return r.LandscapeWideTables &&
		strings.HasPrefix(strings.ToLower(r.orientation), "p") &&
		landscapeW > portraitW &&
		(naturalW > landscapeW || minW > portraitW)
}

// addPageOriented начинает новую страницу заданной ориентации ("P" или "L").
// Размер берётся из формата документа: PageSize(0) отдаёт его в портретных
// координатах, fpdf сам меняет стороны местами для альбома.
func (r *PdfRenderer) addPageOriented(orientation string) {
	wd, ht, _ := r.Pdf.PageSize(0)
	r.Pdf.AddPageFormat(orientation, fpdf.SizeType{Wd: wd, Ht: ht})
}

// enterLandscape отдаёт широкой таблице всю альбомную полосу. Страница
// разворачивается только один раз на группу: соседняя широкая таблица и текст
// между ними продолжаются на той же странице. Сообщает, начата ли страница.
func (r *PdfRenderer) enterLandscape() bool {
	r.inPrelude = false
	r.landscapeTail = false
	r.Pdf.SetRightMargin(r.mright)
	if r.inLandscape {
		return false
	}
	r.addPageOriented("L")
	r.inLandscape = true
	return true
}

// beginLandscapeTail пускает поток дальше по альбомной странице, вместо того
// чтобы бросать её низ пустым. Колонка текста сужается до портретной ширины:
// строки читаются как на остальных страницах, а перенос абзаца через границу
// страниц безопасен — полоса набора одинакова в обеих ориентациях.
//
// Тождество полос — обязательное условие, а не украшение: fpdf считает ширину
// строки один раз на входе в Write/MultiCell, поэтому абзац, начатый на
// альбомной странице и продолженный на портретной, переносится верно только
// пока обе полосы совпадают. Любая иная ширина хвоста молча испортит перенос.
func (r *PdfRenderer) beginLandscapeTail() {
	r.landscapeTail = true
	pageW, pageH := r.Pdf.GetPageSize()
	r.Pdf.SetRightMargin(pageW - pageH + r.mright)
}

// leaveLandscape возвращает документ в портрет. Правое поле восстанавливается
// ДО новой страницы: футер уходящей альбомной страницы рисуется внутри
// AddPageFormat и должен встать по центру всей её ширины.
func (r *PdfRenderer) leaveLandscape() {
	r.inLandscape = false
	r.landscapeTail = false
	r.Pdf.SetRightMargin(r.mright)
}

// acceptPageBreak решает судьбу автоматического разрыва страницы. Своих
// случаев два. Замыкающая линия таблицы и перевод строки после неё имеют
// нулевую высоту, и страница под них — пустая страница: разрыв отклоняется, а
// заведёт страницу первый же настоящий блок. Дозаполняемая альбомная страница,
// наоборот, разрыв заслужила, но fpdf развернул бы ещё одну альбомную, поэтому
// портретную страницу рендерер заводит сам. Во всех прочих случаях решение
// отдаётся fpdf: возвращается его собственный режим авто-разрыва.
//
// Flow:
//
//	```mermaid
//	flowchart TD
//	    A[fpdf: место кончилось] --> B{линия/перевод после таблицы?}
//	    B -- да --> C[разрыв отклонить, страницу не заводить]
//	    B -- нет --> D{дозаполняем альбом?}
//	    D -- нет --> E[вернуть режим авто-разрыва fpdf]
//	    D -- да --> F[вернуть поля, начать портретную]
//	    F --> G[разрыв отклонить: пишем на новой странице]
//	```
func (r *PdfRenderer) acceptPageBreak() bool {
	if r.suppressBreak {
		return false
	}
	if !r.landscapeTail {
		if r.inPrelude && !r.inLandscape {
			// страница, на которой окажется только текст перед широкой
			// таблицей, сразу открывается альбомной — таблица продолжит её
			r.addPage()
			return false
		}
		auto, _ := r.Pdf.GetAutoPageBreak()
		return auto
	}
	// Родной путь разрыва (fpdf.go, CellFormat) сохраняет позицию строки и
	// гасит межсловный интервал перед новой страницей: оператор Tw живёт в
	// потоке страницы, поэтому на уходящей странице он обязан закрыться, иначе
	// растянутые пробелы достанутся её футеру. Перехват обязан повторить это
	// сам — иначе fpdf сделал бы всю уборку, но развернул бы ещё одну
	// альбомную страницу.
	x := r.Pdf.GetX()
	ws := r.Pdf.GetWordSpacing()
	if ws != 0 {
		r.Pdf.SetWordSpacing(0)
	}
	r.leaveLandscape()
	r.addPageOriented("P")
	r.Pdf.SetX(x)
	if ws != 0 {
		r.Pdf.SetWordSpacing(ws)
	}
	return false
}

// addPage начинает новую страницу под продолжение таблицы. Перенос широкой
// таблицы обязан остаться в альбоме, иначе её столбцы, посчитанные под
// альбомную полосу, вылезут за портретную страницу. Обычная таблица, попавшая
// на дозаполняемую альбомную страницу, посчитана под портретную полосу —
// её продолжение уходит в портрет.
func (r *PdfRenderer) addPage() {
	switch {
	case r.inPrelude && !r.inLandscape:
		r.addPageOriented("L")
		r.inLandscape = true
		r.beginLandscapeTail()
	case r.landscapeTail:
		r.leaveLandscape()
		r.addPageOriented("P")
	case r.inLandscape:
		r.addPageOriented("L")
	default:
		r.Pdf.AddPage()
	}
}

// UpdateParagraphStyler - update with default styler
func (r *PdfRenderer) UpdateParagraphStyler(defaultStyler Styler) {
	initcurrent := &containerState{
		listkind:  notlist,
		textStyle: defaultStyler, leftMargin: r.mleft}
	r.cs.push(initcurrent)
}

// UpdateCodeStyler - update code fill styler
func (r *PdfRenderer) UpdateCodeStyler() {
	r.NeedCodeStyleUpdate = true
}

// UpdateBlockquoteStyler - update Blockquote fill styler
func (r *PdfRenderer) UpdateBlockquoteStyler() {
	r.NeedBlockquoteStyleUpdate = true
}

func (r *PdfRenderer) setStyler(s Styler) {
	// see https://github.com/solworktech/md2pdf/issues/18#issuecomment-2179694815
	// This does not address the root cause
	// (https://github.com/solworktech/md2pdf/issues/18#issuecomment-2179694815)
	// but it will correct all cases and is safer.
	if s.Style == "bb" {
		s.Style = "b"
	}
	r.Pdf.SetFont(s.Font, s.Style, s.Size)
	r.Pdf.SetTextColor(s.TextColor.Red, s.TextColor.Green, s.TextColor.Blue)
	r.Pdf.SetFillColor(s.FillColor.Red, s.FillColor.Green, s.FillColor.Blue)
}

func (r *PdfRenderer) write(s Styler, t string) {
	// fmt.Printf("%s, %#v\n",t, s)
	r.Pdf.Write(s.Size+s.Spacing, t)
}

func (r *PdfRenderer) multiCell(s Styler, t string) {
	r.Pdf.MultiCell(0, s.Size+s.Spacing, t, "", "", true)
}

func (r *PdfRenderer) writeLink(s Styler, display, url string) {
	r.Pdf.WriteLinkString(s.Size+s.Spacing, display, url)
}

// RenderNode is a default renderer of a single node of a syntax tree. For
// block nodes it will be called twice: first time with entering=true, second
// time with entering=false, so that it could know when it's working on an open
// tag and when on close. It writes the result to w.
//
// The return value is a way to tell the calling walker to adjust its walk
// pattern: e.g. it can terminate the traversal by returning Terminate. Or it
// can ask the walker to skip a subtree of this node by returning SkipChildren.
// The typical behavior is to return GoToNext, which asks for the usual
// traversal to the next node.
// (above taken verbatim from the blackfriday v2 package)
func (r *PdfRenderer) RenderNode(w io.Writer, node ast.Node, entering bool) ast.WalkStatus {
	if entering && r.landscapePrelude[node] {
		r.inPrelude = true
	}

	switch node := node.(type) {
	case *ast.Text:
		r.processText(node)
	case *ast.Softbreak:
		r.tracer("Softbreak", "Output newline")
		r.cr()
	case *ast.Hardbreak:
		r.tracer("Hardbreak", "Output newline")
		r.cr()
	case *ast.Emph:
		r.processEmph(node, entering)
	case *ast.Strong:
		r.processStrong(node, entering)
	case *ast.Del:
		if entering {
			r.tracer("DEL (entering)", "Not handled")
		} else {
			r.tracer("DEL (leaving)", "Not handled")
		}
	case *ast.HTMLSpan:
		r.processHTMLSpan(node)
	case *ast.Link:
		r.processLink(*node, entering)
	case *ast.Image:
		r.processImage(*node, entering)
	case *ast.Code:
		r.processCode(node)
	case *ast.Document:
		r.tracer("Document", "Not Handled")
	case *ast.Paragraph:
		r.processParagraph(node, entering)
	case *ast.BlockQuote:
		r.processBlockQuote(node, entering)
	case *ast.HTMLBlock:
		r.processHTMLBlock(node)
	case *ast.Heading:
		r.processHeading(node, entering)
	case *ast.HorizontalRule:
		r.processHorizontalRule(node)
	case *ast.List:
		r.processList(*node, entering)
	case *ast.ListItem:
		r.processItem(*node, entering)
	case *ast.CodeBlock:
		r.processCodeblock(*node)
	case *ast.Table:
		r.processTable(node, entering)
	case *ast.TableHeader:
		r.processTableHead(node, entering)
	case *ast.TableBody:
		r.processTableBody(node, entering)
	case *ast.TableRow:
		r.processTableRow(node, entering)
	case *ast.TableCell:
		r.processTableCell(*node, entering)
	case *ast.Math:
		r.processMath(node)
	default:
		fmt.Printf("Unknown node type: %T. Skipping\n", node)
	}
	return ast.GoToNext
}

// RenderHeader is not supported.
func (r *PdfRenderer) RenderHeader(w io.Writer, ast ast.Node) {
	r.tracer("RenderHeader", "Not handled")
}

// RenderFooter is not supported.
func (r *PdfRenderer) RenderFooter(w io.Writer, _ ast.Node) {
}

func (r *PdfRenderer) cr() {
	LH := r.cs.peek().textStyle.Size + r.cs.peek().textStyle.Spacing
	r.tracer("cr()", fmt.Sprintf("LH=%v", LH))
	r.write(r.cs.peek().textStyle, "\n")
}

// Tracer traces parse and pdf generation activity.
func (r *PdfRenderer) tracer(source, msg string) {
	if r.tracerFile != "" {
		indent := strings.Repeat("-", len(r.cs.stack)-1)
		r.w.WriteString(fmt.Sprintf("%v[%v] %v\n", indent, source, msg))
	}
}

func dorect(doc *fpdf.Fpdf, x, y, w, h float64, color Color) {
	doc.SetFillColor(color.Red, color.Green, color.Blue)
	doc.Rect(x, y, w, h, "F")
}

// SetPageBackground - sets background colour of page. String IDs ("blue", "grey", etc) and `Color` structs are both supported
func (r *PdfRenderer) SetPageBackground(colorStr string, color Color) {
	w, h := r.Pdf.GetPageSize()
	if colorStr != "" {
		color = Colorlookup(colorStr)
	}
	dorect(r.Pdf, 0, 0, w, h, color)
}

// Options

// WithUnicodeTranslator configures a unico translator to support characters for latin, russian, etc..
func WithUnicodeTranslator(cp string) RenderOption {
	return func(r *PdfRenderer) {
		r.unicodeTranslator = r.Pdf.UnicodeTranslatorFromDescriptor(cp)
	}
}

// IsHorizontalRuleNewPage if true, will start a new page when encountering a HR (---). Useful for presentations.
func IsHorizontalRuleNewPage(value bool) RenderOption {
	return func(r *PdfRenderer) {
		r.HorizontalRuleNewPage = value
	}
}

// SetSyntaxHighlightBaseDir path to https://github.com/jessp01/gohighlight/tree/master/syntax_files
func SetSyntaxHighlightBaseDir(path string) RenderOption {
	return func(r *PdfRenderer) {
		r.SyntaxHighlightBaseDir = path
	}
}
