package pdf

import (
	"github.com/signintech/gopdf"
)

// Page geometry in points (A4 portrait 595.28 x 841.89).
const (
	pageW        = 595.28
	pageH        = 841.89
	marginX      = 40.0
	marginTop    = 34.0
	contentW     = pageW - 2*marginX
	footerY      = pageH - 36.0
	contentBotY  = footerY - 14.0
	sectionGap   = 20.0
	headingH     = 16.0
	lineThin     = 0.75
	lineRule     = 1.5
	tileGap      = 8.0
	tilePad      = 9.0
	tileH        = 62.0
	legendSwatch = 7.5
	titleRuleY   = 28.0 // rule below a page title, from the top margin
	half         = 2.0
	center       = 0.5 // middle of a slot
	gapXS        = 2.0
	gapS         = 4.0
	gapM         = 6.0
	gapL         = 9.0
)

// Font faces (registered in newDoc) and sizes in points; 9 pt is the smallest text.
const (
	fontRegular  = "inter"
	fontSemiBold = "inter-semibold"
	fontBold     = "inter-bold"

	sizeTitle   = 22.0
	sizePage    = 16.5
	sizeHeading = 12.0
	sizeBody    = 10.0
	sizeSmall   = 9.0
	sizeValue   = 16.5
	sizeTile    = 13.5
)

// Alignments used by doc.text.
const (
	alignLeft   = gopdf.Left | gopdf.Middle
	alignRight  = gopdf.Right | gopdf.Middle
	alignCenter = gopdf.Center | gopdf.Middle
)

// rgb is a text, fill or stroke color.
type rgb struct {
	r, g, b uint8
}

// hex builds an rgb from 0xRRGGBB.
func hex(v uint32) rgb {
	const byteBits, mask = 8, 0xff
	return rgb{
		r: uint8(v >> (byteBits + byteBits) & mask),
		g: uint8(v >> byteBits & mask),
		b: uint8(v & mask),
	}
}

// Print palette (Okabe-Ito based: color-blind safe, series that appear together differ
// in lightness so the charts read in grayscale) and neutrals.
const (
	colorPV          = 0xE69F00
	colorBattery     = 0x009E73
	colorImport      = 0xD55E00
	colorExport      = 0x0072B2
	colorConsumption = 0x5E3C99
	colorSingle      = 0x475569 // single-series charts
	colorInk         = 0x0F172A
	colorMuted       = 0x475569
	colorBody        = 0x334155
	colorRule        = 0xCBD5E1
	colorGrid        = 0xE2E8F0
	colorPanel       = 0xF1F5F9
	colorBrand       = 0x15803D
	colorBetter      = 0x15803D
	colorWorse       = 0xB91C1C
)
