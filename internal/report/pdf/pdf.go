// Package pdf renders a report as an A4 PDF: a summary page with key figures, an energy
// flow page, a patterns and comparison page, a compact table (one row per bucket,
// continued on further pages when needed) and a last page on the data and formulas
// used. Charts are drawn as vectors with print colors
// that are independent of the app theme. Pure Go (works in the scratch image); fonts are
// embedded (Inter, SIL Open Font License, see fonts/OFL.txt).
package pdf

import (
	_ "embed" // fonts
	"fmt"
	"io"
	"time"

	"github.com/signintech/gopdf"

	"github.com/dombyte/solis/internal/report"
)

//go:embed fonts/Inter-Regular.ttf
var interRegular []byte

//go:embed fonts/Inter-SemiBold.ttf
var interSemiBold []byte

//go:embed fonts/Inter-Bold.ttf
var interBold []byte

// Meta is the header information that does not come from the report.
type Meta struct {
	// Generated is the creation time (server local time).
	Generated time.Time
	// Version is the app version.
	Version string
}

// page draws one page of the report.
type page func(d *doc, r report.Report, m Meta)

// Write renders r to w.
func Write(w io.Writer, r report.Report, m Meta) error {
	d, err := newDoc(m)
	if err != nil {
		return err
	}
	pages := []page{summaryPage, flowsPage, patternsPage}
	tables := tablePages(len(r.Buckets))
	total := len(pages) + tables + 1 // + methodology
	for i, p := range pages {
		d.newPage()
		p(d, r, m)
		footer(d, r, i+1, total)
	}
	for i := range tables {
		d.newPage()
		tablePage(d, r, i)
		footer(d, r, len(pages)+i+1, total)
	}
	d.newPage()
	methodPage(d, r, m)
	footer(d, r, total, total)
	if d.err != nil {
		return fmt.Errorf("pdf: draw: %w", d.err)
	}
	if _, err := d.pdf.WriteTo(w); err != nil {
		return fmt.Errorf("pdf: write: %w", err)
	}
	return nil
}

// newDoc starts an A4 document with the embedded fonts.
func newDoc(m Meta) (*doc, error) {
	p := &gopdf.GoPdf{}
	p.Start(gopdf.Config{Unit: gopdf.UnitPT, PageSize: *gopdf.PageSizeA4})
	p.SetInfo(gopdf.PdfInfo{
		Title: "Solis energy report", Creator: "Solis Monitor " + m.Version,
		CreationDate: m.Generated,
	})
	fonts := map[string][]byte{
		fontRegular: interRegular, fontSemiBold: interSemiBold, fontBold: interBold,
	}
	for name, data := range fonts {
		if err := p.AddTTFFontData(name, data); err != nil {
			return nil, fmt.Errorf("pdf: load font %s: %w", name, err)
		}
	}
	return &doc{pdf: p}, nil
}

// box is a rectangle in points from the upper-left corner.
type box struct {
	x, y, w, h float64
}

// doc wraps gopdf with a sticky error so drawing code stays linear: after the first
// failure every call is a no-op and Write reports that error.
type doc struct {
	pdf *gopdf.GoPdf
	err error
}

func (d *doc) newPage() { d.pdf.AddPage() }

func (d *doc) fail(err error) {
	if d.err == nil && err != nil {
		d.err = err
	}
}

// font selects a face and size (pt) and the text color.
func (d *doc) font(face string, size float64, c rgb) {
	d.fail(d.pdf.SetFont(face, "", size))
	d.pdf.SetTextColor(c.r, c.g, c.b)
}

// fit selects face at size, shrunk in half-point steps (down to the body size) until s
// fits into maxW.
func (d *doc) fit(face string, size, maxW float64, s string, c rgb) {
	const step = 0.5
	d.font(face, size, c)
	for size > sizeBody && d.width(s) > maxW {
		size -= step
		d.font(face, size, c)
	}
}

// text writes s into b with align (gopdf.Left|Right|Center plus Top|Middle|Bottom).
func (d *doc) text(b box, s string, align int) {
	if d.err != nil {
		return
	}
	d.pdf.SetXY(b.x, b.y)
	d.fail(d.pdf.CellWithOption(&gopdf.Rect{W: b.w, H: b.h}, s, gopdf.CellOption{Align: align}))
}

// width measures s in the current font.
func (d *doc) width(s string) float64 {
	w, err := d.pdf.MeasureTextWidth(s)
	d.fail(err)
	return w
}

// rect fills b with c.
func (d *doc) rect(b box, c rgb) {
	if b.w <= 0 || b.h <= 0 {
		return
	}
	d.pdf.SetFillColor(c.r, c.g, c.b)
	d.pdf.RectFromUpperLeftWithStyle(b.x, b.y, b.w, b.h, "F")
}

// frame strokes b with c (hairline borders of tiles and tables).
func (d *doc) frame(b box, c rgb) {
	d.stroke(c, lineThin)
	d.pdf.RectFromUpperLeftWithStyle(b.x, b.y, b.w, b.h, "D")
}

// line draws from (x1, y1) to (x2, y2) with the current stroke.
func (d *doc) line(x1, y1, x2, y2 float64) { d.pdf.Line(x1, y1, x2, y2) }

// stroke sets the stroke color and width.
func (d *doc) stroke(c rgb, width float64) {
	d.pdf.SetStrokeColor(c.r, c.g, c.b)
	d.pdf.SetLineWidth(width)
}
