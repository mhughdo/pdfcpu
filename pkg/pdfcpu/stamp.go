/*
Copyright 2018 The pdfcpu Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pdfcpu

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/filter"
	"github.com/pdfcpu/pdfcpu/pkg/log"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/fonts/metrics"

	"github.com/pkg/errors"
)

const (
	degToRad = math.Pi / 180
	radToDeg = 180 / math.Pi
)

// Rotation along one of 2 diagonals
const (
	noDiagonal = iota
	diagonalLLToUR
	diagonalULToLR
)

// Render mode
const (
	rmFill = iota
	rmStroke
	rmFillAndStroke
)

var (
	errNoContent   = errors.New("pdfcpu: stamp: PDF page has no content")
	errNoWatermark = errors.Errorf("pdfcpu: no watermarks found - nothing removed")
)

type watermarkParamMap map[string]func(string, *Watermark) error

// Handle applies parameter completion and if successful
// parses the parameter values into import.
func (m watermarkParamMap) Handle(paramPrefix, paramValueStr string, imp *Watermark) error {

	var param string

	// Completion support
	for k := range m {
		if !strings.HasPrefix(k, paramPrefix) {
			continue
		}
		if len(param) > 0 {
			return errors.Errorf("pdfcpu: ambiguous parameter prefix \"%s\"", paramPrefix)
		}
		param = k
	}

	if param == "" {
		return errors.Errorf("pdfcpu: unknown parameter prefix \"%s\"", paramPrefix)
	}

	return m[param](paramValueStr, imp)
}

var wmParamMap = watermarkParamMap{
	"fontname":    parseFontName,
	"points":      parseFontSize,
	"color":       parseColor,
	"rotation":    parseRotation,
	"diagonal":    parseDiagonal,
	"opacity":     parseOpacity,
	"mode":        parseRenderMode,
	"rendermode":  parseRenderMode,
	"position":    parsePositionAnchorWM,
	"offset":      parsePositionOffsetWM,
	"scalefactor": parseScaleFactorWM,
}

type matrix [3][3]float64

var identMatrix = matrix{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}

func (m matrix) multiply(n matrix) matrix {
	var p matrix
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				p[i][j] += m[i][k] * n[k][j]
			}
		}
	}
	return p
}

func (m matrix) String() string {
	return fmt.Sprintf("%3.2f %3.2f %3.2f\n%3.2f %3.2f %3.2f\n%3.2f %3.2f %3.2f\n",
		m[0][0], m[0][1], m[0][2],
		m[1][0], m[1][1], m[1][2],
		m[2][0], m[2][1], m[2][2])
}

// SimpleColor is a simple rgb wrapper.
type SimpleColor struct {
	r, g, b float32 // intensities between 0 and 1.
}

func (sc SimpleColor) String() string {
	return fmt.Sprintf("r=%1.1f g=%1.1f b=%1.1f", sc.r, sc.g, sc.b)
}

type formCache map[*Rectangle]*IndirectRef

// Watermark represents the basic structure and command details for the commands "Stamp" and "Watermark".
type Watermark struct {

	// configuration
	TextString        string      // raw display text.
	TextLines         []string    // display multiple lines of text.
	FileName          string      // display pdf page or png image.
	Page              int         // the page number of a PDF file.
	OnTop             bool        // if true this is a STAMP else this is a WATERMARK.
	Pos               anchor      // position anchor, one of tl,tc,tr,l,c,r,bl,bc,br,full.
	Dx, Dy            int         // anchor offset.
	FontName          string      // supported are Adobe base fonts only. (as of now: Helvetica, Times-Roman, Courier)
	FontSize          int         // font scaling factor.
	ScaledFontSize    int         // font scaling factor for a specific page
	Color             SimpleColor // fill color(=non stroking color).
	Rotation          float64     // rotation to apply in degrees. -180 <= x <= 180
	Diagonal          int         // paint along the diagonal.
	UserRotOrDiagonal bool        // true if one of rotation or diagonal provided overriding the default.
	Opacity           float64     // opacity the displayed text. 0 <= x <= 1
	RenderMode        int         // fill=0, stroke=1 fill&stroke=2
	Scale             float64     // relative scale factor. 0 <= x <= 1
	ScaleAbs          bool        // true for absolute scaling.
	Update            bool        // true for updating instead of adding a page watermark.

	// resources
	ocg, extGState, font, image *IndirectRef

	// for an image or PDF watermark
	width, height int // image or page dimensions.

	// for a PDF watermark
	resDict *IndirectRef // page resource dict.
	cs      []byte       // page content stream.

	// page specific
	bb      *Rectangle   // bounding box of the form representing this watermark.
	vp      *Rectangle   // page dimensions for text alignment.
	pageRot float64      // page rotation in effect.
	form    *IndirectRef // Forms are dependent on given page dimensions.

	// house keeping
	objs   IntSet    // objects for which wm has been applied already.
	fCache formCache // form cache.
}

// DefaultWatermarkConfig returns the default configuration.
func DefaultWatermarkConfig() *Watermark {
	return &Watermark{
		Page:       1,
		FontName:   "Helvetica",
		FontSize:   24,
		Pos:        Center,
		Scale:      0.5,
		ScaleAbs:   false,
		Color:      SimpleColor{0.5, 0.5, 0.5}, // gray
		Diagonal:   diagonalLLToUR,
		Opacity:    1.0,
		RenderMode: rmFill,
		objs:       IntSet{},
		fCache:     formCache{},
		TextLines:  []string{},
	}
}

func (wm Watermark) typ() string {
	if wm.isImage() {
		return "image"
	}
	if wm.isPDF() {
		return "pdf"
	}
	return "text"
}

func (wm Watermark) String() string {

	var s string
	if !wm.OnTop {
		s = "not "
	}

	t := wm.TextString
	if len(t) == 0 {
		t = wm.FileName
	}

	sc := "relative"
	if wm.ScaleAbs {
		sc = "absolute"
	}

	bbox := ""
	if wm.bb != nil {
		bbox = (*wm.bb).String()
	}

	vp := ""
	if wm.vp != nil {
		vp = (*wm.vp).String()
	}

	return fmt.Sprintf("Watermark: <%s> is %son top, typ:%s\n"+
		"%s %d points\n"+
		"PDFpage#: %d\n"+
		"scaling: %.1f %s\n"+
		"color: %s\n"+
		"rotation: %.1f\n"+
		"diagonal: %d\n"+
		"opacity: %.1f\n"+
		"renderMode: %d\n"+
		"bbox:%s\n"+
		"vp:%s\n"+
		"pageRotation: %.1f\n",
		t, s, wm.typ(),
		wm.FontName, wm.FontSize,
		wm.Page,
		wm.Scale, sc,
		wm.Color,
		wm.Rotation,
		wm.Diagonal,
		wm.Opacity,
		wm.RenderMode,
		bbox,
		vp,
		wm.pageRot,
	)
}

// OnTopString returns "watermark" or "stamp" whichever applies.
func (wm Watermark) OnTopString() string {
	s := "watermark"
	if wm.OnTop {
		s = "stamp"
	}
	return s
}

func (wm Watermark) calcMaxTextWidth() float64 {

	var maxWidth float64

	for _, l := range wm.TextLines {
		w := metrics.TextWidth(l, wm.FontName, wm.ScaledFontSize)
		if w > maxWidth {
			maxWidth = w
		}
	}

	return maxWidth
}

func parsePositionAnchorWM(s string, wm *Watermark) error {

	switch s {
	case "tl":
		wm.Pos = TopLeft
	case "tc":
		wm.Pos = TopCenter
	case "tr":
		wm.Pos = TopRight
	case "l":
		wm.Pos = Left
	case "c":
		wm.Pos = Center
	case "r":
		wm.Pos = Right
	case "bl":
		wm.Pos = BottomLeft
	case "bc":
		wm.Pos = BottomCenter
	case "br":
		wm.Pos = BottomRight
	default:
		return errors.Errorf("pdfcpu: unknown position anchor: %s", s)
	}

	return nil
}

func parsePositionOffsetWM(s string, wm *Watermark) error {

	var err error

	d := strings.Split(s, " ")
	if len(d) != 2 {
		return errors.Errorf("pdfcpu: illegal position offset string: need 2 numeric values, %s\n", s)
	}

	wm.Dx, err = strconv.Atoi(d[0])
	if err != nil {
		return err
	}

	wm.Dy, err = strconv.Atoi(d[1])
	if err != nil {
		return err
	}

	return nil
}

func parseScaleFactorWM(s string, wm *Watermark) (err error) {
	wm.Scale, wm.ScaleAbs, err = parseScaleFactor(s)
	return err
}

func parseFontName(s string, wm *Watermark) error {
	if !supportedWatermarkFont(s) {
		return errors.Errorf("pdfcpu: %s is unsupported, try one of Helvetica, Times-Roman, Courier.\n", s)
	}
	wm.FontName = s
	return nil
}

func parseFontSize(s string, wm *Watermark) error {

	fs, err := strconv.Atoi(s)
	if err != nil {
		return err
	}

	wm.FontSize = fs

	return nil
}

func parseScaleFactor(s string) (float64, bool, error) {

	ss := strings.Split(s, " ")
	if len(ss) > 2 {
		return 0, false, errors.Errorf("illegal scale string: 0.0 <= i <= 1.0 {abs|rel}, %s\n", s)
	}

	sc, err := strconv.ParseFloat(ss[0], 64)
	if err != nil {
		return 0, false, errors.Errorf("scale factor must be a float value: %s\n", ss[0])
	}
	if sc < 0 || sc > 1 {
		return 0, false, errors.Errorf("illegal scale factor: 0.0 <= s <= 1.0, %s\n", ss[0])
	}

	var scaleAbs bool

	if len(ss) == 2 {
		switch ss[1] {
		case "a", "abs":
			scaleAbs = true

		case "r", "rel":
			scaleAbs = false

		default:
			return 0, false, errors.Errorf("illegal scale mode: abs|rel, %s\n", ss[1])
		}
	}

	return sc, scaleAbs, nil
}

func parseColor(s string, wm *Watermark) error {

	cs := strings.Split(s, " ")
	if len(cs) != 3 {
		return errors.Errorf("pdfcpu: illegal color string: 3 intensities 0.0 <= i <= 1.0, %s\n", s)
	}

	r, err := strconv.ParseFloat(cs[0], 32)
	if err != nil {
		return errors.Errorf("red must be a float value: %s\n", cs[0])
	}
	if r < 0 || r > 1 {
		return errors.New("pdfcpu: red: a color value is an intensity between 0.0 and 1.0")
	}
	wm.Color.r = float32(r)

	g, err := strconv.ParseFloat(cs[1], 32)
	if err != nil {
		return errors.Errorf("pdfcpu: green must be a float value: %s\n", cs[1])
	}
	if g < 0 || g > 1 {
		return errors.New("pdfcpu: green: a color value is an intensity between 0.0 and 1.0")
	}
	wm.Color.g = float32(g)

	b, err := strconv.ParseFloat(cs[2], 32)
	if err != nil {
		return errors.Errorf("pdfcpu: blue must be a float value: %s\n", cs[2])
	}
	if b < 0 || b > 1 {
		return errors.New("pdfcpu: blue: a color value is an intensity between 0.0 and 1.0")
	}
	wm.Color.b = float32(b)

	return nil
}

func parseRotation(s string, wm *Watermark) error {

	if wm.UserRotOrDiagonal {
		return errors.New("pdfcpu: please specify rotation or diagonal (r or d)")
	}

	r, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return errors.Errorf("pdfcpu: rotation must be a float value: %s\n", s)
	}
	if r < -180 || r > 180 {
		return errors.Errorf("pdfcpu: illegal rotation: -180 <= r <= 180 degrees, %s\n", s)
	}

	wm.Rotation = r
	wm.Diagonal = noDiagonal
	wm.UserRotOrDiagonal = true

	return nil
}

func parseDiagonal(s string, wm *Watermark) error {

	if wm.UserRotOrDiagonal {
		return errors.New("pdfcpu: please specify rotation or diagonal (r or d)")
	}

	d, err := strconv.Atoi(s)
	if err != nil {
		return errors.Errorf("pdfcpu: illegal diagonal value: allowed 1 or 2, %s\n", s)
	}
	if d != diagonalLLToUR && d != diagonalULToLR {
		return errors.New("pdfcpu: diagonal: 1..lower left to upper right, 2..upper left to lower right")
	}

	wm.Diagonal = d
	wm.Rotation = 0
	wm.UserRotOrDiagonal = true

	return nil
}

func parseOpacity(s string, wm *Watermark) error {

	o, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return errors.Errorf("pdfcpu: opacity must be a float value: %s\n", s)
	}
	if o < 0 || o > 1 {
		return errors.Errorf("pdfcpu: illegal opacity: 0.0 <= r <= 1.0, %s\n", s)
	}
	wm.Opacity = o

	return nil
}

func parseRenderMode(s string, wm *Watermark) error {

	m, err := strconv.Atoi(s)
	if err != nil {
		return errors.Errorf("pdfcpu: illegal render mode value: allowed 0,1,2, %s\n", s)
	}
	if m != rmFill && m != rmStroke && m != rmFillAndStroke {
		return errors.New("pdfcpu: valid rendermodes: 0..fill, 1..stroke, 2..fill&stroke")
	}
	wm.RenderMode = m

	return nil
}

// ParseWatermarkDetails parses a Watermark/Stamp command string into an internal structure.
func ParseWatermarkDetails(s string, onTop bool) (*Watermark, error) {

	wm := DefaultWatermarkConfig()
	wm.OnTop = onTop

	ss := strings.Split(s, ",")

	setWatermarkType(ss[0], wm)

	if len(ss) == 1 {
		return wm, nil
	}

	for _, s := range ss[1:] {

		ss1 := strings.Split(s, ":")
		if len(ss1) != 2 {
			return nil, parseWatermarkError(onTop)
		}

		paramPrefix := strings.TrimSpace(ss1[0])
		paramValueStr := strings.TrimSpace(ss1[1])

		if err := wmParamMap.Handle(paramPrefix, paramValueStr, wm); err != nil {
			return nil, err
		}
	}

	return wm, nil
}

func (wm Watermark) calcMinFontSize(w float64) int {

	var minSize int

	for _, l := range wm.TextLines {
		w := metrics.FontSize(l, wm.FontName, w)
		if minSize == 0.0 {
			minSize = w
		}
		if w < minSize {
			minSize = w
		}
	}

	return minSize
}

// IsPDF returns whether the watermark content is an image or text.
func (wm Watermark) isPDF() bool {
	return len(wm.FileName) > 0 && strings.ToLower(filepath.Ext(wm.FileName)) == ".pdf"
}

// IsImage returns whether the watermark content is an image or text.
func (wm Watermark) isImage() bool {
	return len(wm.FileName) > 0 && strings.ToLower(filepath.Ext(wm.FileName)) != ".pdf"
}

func (wm *Watermark) calcBoundingBox() {

	var bb *Rectangle

	if wm.isImage() || wm.isPDF() {

		bb = RectForDim(float64(wm.width), float64(wm.height))
		ar := bb.AspectRatio()

		if wm.ScaleAbs {
			bb.UR.X = wm.Scale * bb.Width()
			bb.UR.Y = bb.UR.X / ar

			wm.bb = bb
			return
		}

		if ar >= 1 {
			bb.UR.X = wm.Scale * wm.vp.Width()
			bb.UR.Y = bb.UR.X / ar
		} else {
			bb.UR.Y = wm.Scale * wm.vp.Height()
			bb.UR.X = bb.UR.Y * ar
		}

		wm.bb = bb
		return
	}

	// font watermark

	var w float64
	if wm.ScaleAbs {
		wm.ScaledFontSize = int(float64(wm.FontSize) * wm.Scale)
		w = wm.calcMaxTextWidth()
	} else {
		w = wm.Scale * wm.vp.Width()
		wm.ScaledFontSize = wm.calcMinFontSize(w)
	}
	h := float64(len(wm.TextLines)) * float64(wm.ScaledFontSize)
	wm.bb = Rect(0, 0, w, h)

	return
}

func (wm *Watermark) calcTransformMatrix() *matrix {

	var sin, cos float64
	r := wm.Rotation

	if wm.Diagonal != noDiagonal {

		// Calculate the angle of the diagonal with respect of the aspect ratio of the bounding box.
		r = math.Atan(wm.vp.Height()/wm.vp.Width()) * float64(radToDeg)

		if wm.bb.AspectRatio() < 1 {
			r -= 90
		}

		if wm.Diagonal == diagonalULToLR {
			r = -r
		}

	}

	// Apply negative page rotation.
	r += wm.pageRot

	sin = math.Sin(float64(r) * float64(degToRad))
	cos = math.Cos(float64(r) * float64(degToRad))

	// 1) Rotate
	m1 := identMatrix
	m1[0][0] = cos
	m1[0][1] = sin
	m1[1][0] = -sin
	m1[1][1] = cos

	// 2) Translate
	m2 := identMatrix

	var dy float64
	if !wm.isImage() && !wm.isPDF() {
		dy = wm.bb.LL.Y
	}

	ll := lowerLeftCorner(wm.vp.Width(), wm.vp.Height(), wm.bb.Width(), wm.bb.Height(), wm.Pos)
	m2[2][0] = ll.X + wm.bb.Width()/2 + float64(wm.Dx) + sin*(wm.bb.Height()/2+dy) - cos*wm.bb.Width()/2
	m2[2][1] = ll.Y + wm.bb.Height()/2 + float64(wm.Dy) - cos*(wm.bb.Height()/2+dy) - sin*wm.bb.Width()/2

	m := m1.multiply(m2)
	return &m
}

func onTopString(onTop bool) string {
	e := "watermark"
	if onTop {
		e = "stamp"
	}
	return e
}

func parseWatermarkError(onTop bool) error {
	s := onTopString(onTop)
	return errors.Errorf("Invalid %s configuration string. Please consult pdfcpu help %s.\n", s, s)
}

func setWatermarkType(s string, wm *Watermark) error {

	ss := strings.Split(s, ":")

	wm.TextString = ss[0]

	for _, l := range strings.Split(ss[0], `\n`) {
		wm.TextLines = append(wm.TextLines, l)
	}

	if len(wm.TextLines) > 1 {
		// Multiline text watermark.
		return nil
	}

	ext := strings.ToLower(filepath.Ext(ss[0]))
	if MemberOf(ext, []string{".jpg", ".jpeg", ".png", ".tif", ".tiff", ".pdf"}) {
		wm.FileName = wm.TextString
	}

	if len(ss) > 1 {
		// Parse page number for PDF watermarks.
		var err error
		wm.Page, err = strconv.Atoi(ss[1])
		if err != nil {
			return errors.Errorf("illegal page number value: %s\n", ss[1])
		}
	}

	return nil
}

func supportedWatermarkFont(fn string) bool {
	for _, s := range metrics.FontNames() {
		if fn == s {
			return true
		}
	}
	return false
}

func createFontResForWM(xRefTable *XRefTable, wm *Watermark) error {

	d := NewDict()
	d.InsertName("Type", "Font")
	d.InsertName("Subtype", "Type1")
	d.InsertName("BaseFont", wm.FontName)

	ir, err := xRefTable.IndRefForNewObject(d)
	if err != nil {
		return err
	}

	wm.font = ir

	return nil
}

func contentStream(xRefTable *XRefTable, o Object) ([]byte, error) {

	o, err := xRefTable.Dereference(o)
	if err != nil {
		return nil, err
	}

	bb := []byte{}

	switch o := o.(type) {

	case StreamDict:

		// Decode streamDict for supported filters only.
		err := decodeStream(&o)
		if err == filter.ErrUnsupportedFilter {
			return nil, errors.New("pdfcpu: unsupported filter: unable to decode content for PDF watermark")
		}
		if err != nil {
			return nil, err
		}

		bb = o.Content

	case Array:
		for _, o := range o {

			if o == nil {
				continue
			}

			sd, err := xRefTable.DereferenceStreamDict(o)
			if err != nil {
				return nil, err
			}

			// Decode streamDict for supported filters only.
			err = decodeStream(sd)
			if err == filter.ErrUnsupportedFilter {
				return nil, errors.New("pdfcpu: unsupported filter: unable to decode content for PDF watermark")
			}
			if err != nil {
				return nil, err
			}

			bb = append(bb, sd.Content...)
		}
	}

	if len(bb) == 0 {
		return nil, errNoContent
	}

	return bb, nil
}

func identifyObjNrs(ctx *Context, o Object, objNrs IntSet) error {

	switch o := o.(type) {

	case IndirectRef:

		objNrs[o.ObjectNumber.Value()] = true

		o1, err := ctx.Dereference(o)
		if err != nil {
			return err
		}

		err = identifyObjNrs(ctx, o1, objNrs)
		if err != nil {
			return err
		}

	case Dict:
		for _, v := range o {
			err := identifyObjNrs(ctx, v, objNrs)
			if err != nil {
				return err
			}
		}

	case StreamDict:
		for _, v := range o.Dict {
			err := identifyObjNrs(ctx, v, objNrs)
			if err != nil {
				return err
			}
		}

	case Array:
		for _, v := range o {
			err := identifyObjNrs(ctx, v, objNrs)
			if err != nil {
				return err
			}
		}

	}

	return nil
}

func migrateObject(ctxSource, ctxDest *Context, o Object) error {

	// Identify involved objNrs.
	objNrs := IntSet{}
	err := identifyObjNrs(ctxSource, o, objNrs)
	if err != nil {
		return err
	}

	// Create a lookup table mapping objNrs from ctxSource to ctxDest.
	// Create lookup table for object numbers.
	// The first number is the successor of the last number in ctxDest.
	lookup := lookupTable(objNrs, *ctxDest.Size)

	// Patch indRefs of resourceDict.
	patchObject(o, lookup)

	// Patch all involved indRefs.
	for i := range lookup {
		patchObject(ctxSource.Table[i].Object, lookup)
	}

	// Migrate xrefTableEntries.
	for k, v := range lookup {
		ctxDest.Table[v] = ctxSource.Table[k]
		*ctxDest.Size++
	}

	return nil
}

func createPDFResForWM(ctx *Context, wm *Watermark) error {

	xRefTable := ctx.XRefTable

	// This PDF file is assumed to be valid.
	otherCtx, err := ReadFile(wm.FileName, NewDefaultConfiguration())
	if err != nil {
		return err
	}

	otherXRefTable := otherCtx.XRefTable

	d, inhPAttrs, err := otherXRefTable.PageDict(wm.Page)
	if err != nil {
		return err
	}
	if d == nil {
		return errors.Errorf("pdfcpu: unknown page number: %d\n", wm.Page)
	}

	// Retrieve content stream bytes.

	o, found := d.Find("Contents")
	if !found {
		return errors.New("pdfcpu: PDF page has no content")
	}

	wm.cs, err = contentStream(otherXRefTable, o)
	if err != nil {
		return err
	}

	// Migrate all objects referenced by this external resource dict into this context.
	err = migrateObject(otherCtx, ctx, inhPAttrs.resources)
	if err != nil {
		return err
	}

	// Create an object for this resDict in xRefTable.
	ir, err := xRefTable.IndRefForNewObject(inhPAttrs.resources)
	if err != nil {
		return err
	}

	wm.resDict = ir

	vp := viewPort(xRefTable, inhPAttrs)

	wm.width = int(vp.Width())
	wm.height = int(vp.Height())

	return nil
}

func createImageResource(xRefTable *XRefTable, r io.Reader) (*IndirectRef, int, int, error) {

	bb, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, 0, 0, err
	}

	var sd *StreamDict
	r = bytes.NewReader(bb)

	// We identify JPG via its magic bytes.
	if bytes.HasPrefix(bb, []byte("\xff\xd8")) {
		// Process JPG by wrapping byte stream into DCTEncoded object stream.
		c, _, err := image.DecodeConfig(r)
		if err != nil {
			return nil, 0, 0, err
		}

		sd, err = ReadJPEG(xRefTable, bb, c)
		if err != nil {
			return nil, 0, 0, err
		}

	} else {
		// Process other formats by decoding into an image
		// and subsequent object stream encoding,
		img, _, err := image.Decode(r)
		if err != nil {
			return nil, 0, 0, err
		}

		sd, err = imgToImageDict(xRefTable, img)
		if err != nil {
			return nil, 0, 0, err
		}
	}

	w := *sd.IntEntry("Width")
	h := *sd.IntEntry("Height")

	indRef, err := xRefTable.IndRefForNewObject(*sd)
	if err != nil {
		return nil, 0, 0, err
	}

	return indRef, w, h, nil
}

func createImageResForWM(xRefTable *XRefTable, wm *Watermark) (err error) {

	f, err := os.Open(wm.FileName)
	if err != nil {
		return err
	}
	defer f.Close()

	wm.image, wm.width, wm.height, err = createImageResource(xRefTable, f)

	return err
}

func createResourcesForWM(ctx *Context, wm *Watermark) error {

	xRefTable := ctx.XRefTable

	if wm.isPDF() {
		return createPDFResForWM(ctx, wm)
	}

	if wm.isImage() {
		return createImageResForWM(xRefTable, wm)
	}

	return createFontResForWM(xRefTable, wm)
}

func ensureOCG(xRefTable *XRefTable, wm *Watermark) error {

	name := "Background"
	subt := "BG"
	if wm.OnTop {
		name = "Watermark"
		subt = "FG"
	}

	d := Dict(
		map[string]Object{
			"Name": StringLiteral(name),
			"Type": Name("OCG"),
			"Usage": Dict(
				map[string]Object{
					"PageElement": Dict(map[string]Object{"Subtype": Name(subt)}),
					"View":        Dict(map[string]Object{"ViewState": Name("ON")}),
					"Print":       Dict(map[string]Object{"PrintState": Name("ON")}),
					"Export":      Dict(map[string]Object{"ExportState": Name("ON")}),
				},
			),
		},
	)

	ir, err := xRefTable.IndRefForNewObject(d)
	if err != nil {
		return err
	}

	wm.ocg = ir

	return nil
}

func prepareOCPropertiesInRoot(ctx *Context, wm *Watermark) error {

	rootDict, err := ctx.Catalog()
	if err != nil {
		return err
	}

	if o, ok := rootDict.Find("OCProperties"); ok {

		// Set wm.ocg indRef
		d, err := ctx.DereferenceDict(o)
		if err != nil {
			return err
		}

		o, found := d.Find("OCGs")
		if found {
			a, err := ctx.DereferenceArray(o)
			if err != nil {
				return errors.Errorf("OCProperties: corrupt OCGs element")
			}

			ir, ok := a[0].(IndirectRef)
			if !ok {
				return errors.Errorf("OCProperties: corrupt OCGs element")
			}
			wm.ocg = &ir

			return nil
		}
	}

	if err := ensureOCG(ctx.XRefTable, wm); err != nil {
		return err
	}

	optionalContentConfigDict := Dict(
		map[string]Object{
			"AS": Array{
				Dict(
					map[string]Object{
						"Category": NewNameArray("View"),
						"Event":    Name("View"),
						"OCGs":     Array{*wm.ocg},
					},
				),
				Dict(
					map[string]Object{
						"Category": NewNameArray("Print"),
						"Event":    Name("Print"),
						"OCGs":     Array{*wm.ocg},
					},
				),
				Dict(
					map[string]Object{
						"Category": NewNameArray("Export"),
						"Event":    Name("Export"),
						"OCGs":     Array{*wm.ocg},
					},
				),
			},
			"ON":       Array{*wm.ocg},
			"Order":    Array{},
			"RBGroups": Array{},
		},
	)

	d := Dict(
		map[string]Object{
			"OCGs": Array{*wm.ocg},
			"D":    optionalContentConfigDict,
		},
	)

	rootDict.Update("OCProperties", d)
	return nil
}

func createFormResDict(xRefTable *XRefTable, wm *Watermark) (*IndirectRef, error) {

	if wm.isPDF() {
		//return consolidated ResDict of for PDF/PageBoxColorInfo
		return wm.resDict, nil
	}

	if wm.isImage() {

		d := Dict(
			map[string]Object{
				"ProcSet": NewNameArray("PDF", "ImageC"),
				"XObject": Dict(map[string]Object{"Im0": *wm.image}),
			},
		)

		return xRefTable.IndRefForNewObject(d)

	}

	d := Dict(
		map[string]Object{
			"Font":    Dict(map[string]Object{wm.FontName: *wm.font}),
			"ProcSet": NewNameArray("PDF", "Text"),
		},
	)

	return xRefTable.IndRefForNewObject(d)
}

func createForm(xRefTable *XRefTable, wm *Watermark, withBB bool) error {

	// The forms bounding box is dependent on the page dimensions.
	wm.calcBoundingBox()
	bb := wm.bb

	// Cache the form for every bounding box encountered.
	ir, ok := wm.fCache[bb] // *bb ??
	if ok {
		wm.form = ir
		return nil
	}

	var b bytes.Buffer

	if wm.isPDF() {
		fmt.Fprintf(&b, "%f 0 0 %f 0 0 cm ", bb.Width()/wm.vp.Width(), bb.Height()/wm.vp.Height())
		_, err := b.Write(wm.cs)
		if err != nil {
			return err
		}
	} else if wm.isImage() {
		fmt.Fprintf(&b, "q %f 0 0 %f 0 0 cm /Im0 Do Q", bb.Width(), bb.Height()) // TODO dont need Q
	} else {

		// 12 font points result in a vertical displacement of 9.47
		dy := -float64(wm.ScaledFontSize) / 12 * 9.47

		wmForm := "0 g 0 G 0 i 0 J []0 d 0 j 1 w 10 M 0 Tc 0 Tw 100 Tz 0 TL %d Tr 0 Ts "
		fmt.Fprintf(&b, wmForm, wm.RenderMode)

		j := 1
		for i := len(wm.TextLines) - 1; i >= 0; i-- {

			sw := metrics.TextWidth(wm.TextLines[i], wm.FontName, wm.ScaledFontSize)
			dx := wm.bb.Width()/2 - sw/2

			fmt.Fprintf(&b, "BT /%s %d Tf %f %f %f rg %f %f Td (%s) Tj ET ",
				wm.FontName, wm.ScaledFontSize, wm.Color.r, wm.Color.g, wm.Color.b, dx, dy+float64(j*wm.ScaledFontSize), wm.TextLines[i])
			j++
		}

	}

	// Paint bounding box
	if withBB {
		fmt.Fprintf(&b, "[]0 d 2 w %f %f m %f %f l %f %f l %f %f l s ",
			bb.LL.X, bb.LL.Y,
			bb.UR.X, bb.LL.Y,
			bb.UR.X, bb.UR.Y,
			bb.LL.X, bb.UR.Y,
		)
	}

	ir, err := createFormResDict(xRefTable, wm)
	if err != nil {
		return err
	}

	sd := StreamDict{
		Dict: Dict(
			map[string]Object{
				"Type":      Name("XObject"),
				"Subtype":   Name("Form"),
				"BBox":      bb.Array(),
				"Matrix":    NewIntegerArray(1, 0, 0, 1, 0, 0),
				"OC":        *wm.ocg,
				"Resources": *ir,
			},
		),
		Content: b.Bytes(),
	}

	err = encodeStream(&sd)
	if err != nil {
		return err
	}

	ir, err = xRefTable.IndRefForNewObject(sd)
	if err != nil {
		return err
	}

	wm.fCache[wm.bb] = ir
	wm.form = ir

	return nil
}

func createExtGStateForStamp(xRefTable *XRefTable, wm *Watermark) error {

	d := Dict(
		map[string]Object{
			"Type": Name("ExtGState"),
			"CA":   Float(wm.Opacity),
			"ca":   Float(wm.Opacity),
		},
	)

	ir, err := xRefTable.IndRefForNewObject(d)
	if err != nil {
		return err
	}

	wm.extGState = ir

	return nil
}

func insertPageResourcesForWM(xRefTable *XRefTable, pageDict Dict, wm *Watermark, gsID, xoID string) error {

	resourceDict := Dict(
		map[string]Object{
			"ExtGState": Dict(map[string]Object{gsID: *wm.extGState}),
			"XObject":   Dict(map[string]Object{xoID: *wm.form}),
		},
	)

	pageDict.Insert("Resources", resourceDict)

	return nil
}

func updatePageResourcesForWM(xRefTable *XRefTable, resDict Dict, wm *Watermark, gsID, xoID *string) error {

	o, ok := resDict.Find("ExtGState")
	if !ok {
		resDict.Insert("ExtGState", Dict(map[string]Object{*gsID: *wm.extGState}))
	} else {
		d, _ := xRefTable.DereferenceDict(o)
		for i := 0; i < 1000; i++ {
			*gsID = "GS" + strconv.Itoa(i)
			if _, found := d.Find(*gsID); !found {
				break
			}
		}
		d.Insert(*gsID, *wm.extGState)
	}

	o, ok = resDict.Find("XObject")
	if !ok {
		resDict.Insert("XObject", Dict(map[string]Object{*xoID: *wm.form}))
	} else {
		d, _ := xRefTable.DereferenceDict(o)
		for i := 0; i < 1000; i++ {
			*xoID = "Fm" + strconv.Itoa(i)
			if _, found := d.Find(*xoID); !found {
				break
			}
		}
		d.Insert(*xoID, *wm.form)
	}

	return nil
}

func wmContent(wm *Watermark, gsID, xoID string) []byte {

	m := wm.calcTransformMatrix()

	insertOCG := " /Artifact <</Subtype /Watermark /Type /Pagination >>BDC q %.2f %.2f %.2f %.2f %.2f %.2f cm /%s gs /%s Do Q EMC "

	var b bytes.Buffer
	fmt.Fprintf(&b, insertOCG, m[0][0], m[0][1], m[1][0], m[1][1], m[2][0], m[2][1], gsID, xoID)

	return b.Bytes()
}

func insertPageContentsForWM(xRefTable *XRefTable, pageDict Dict, wm *Watermark, gsID, xoID string) error {

	sd := &StreamDict{Dict: NewDict()}

	sd.Content = wmContent(wm, gsID, xoID)

	err := encodeStream(sd)
	if err != nil {
		return err
	}

	ir, err := xRefTable.IndRefForNewObject(*sd)
	if err != nil {
		return err
	}

	pageDict.Insert("Contents", *ir)

	return nil
}

func updatePageContentsForWM(xRefTable *XRefTable, obj Object, wm *Watermark, gsID, xoID string) error {

	var entry *XRefTableEntry
	var objNr int

	ir, ok := obj.(IndirectRef)
	if ok {
		objNr = ir.ObjectNumber.Value()
		if wm.objs[objNr] {
			// wm already applied to this content stream.
			return nil
		}
		genNr := ir.GenerationNumber.Value()
		entry, _ = xRefTable.FindTableEntry(objNr, genNr)
		obj = entry.Object
	}

	switch o := obj.(type) {

	case StreamDict:

		err := patchContentForWM(&o, gsID, xoID, wm, true)
		if err != nil {
			return err
		}

		entry.Object = o
		wm.objs[objNr] = true

	case Array:

		// Get stream dict for first element.
		o1 := o[0]
		ir, _ := o1.(IndirectRef)
		objNr = ir.ObjectNumber.Value()
		genNr := ir.GenerationNumber.Value()
		entry, _ := xRefTable.FindTableEntry(objNr, genNr)
		sd, _ := (entry.Object).(StreamDict)

		if len(o) == 1 || !wm.OnTop {

			if wm.objs[objNr] {
				// wm already applied to this content stream.
				return nil
			}

			err := patchContentForWM(&sd, gsID, xoID, wm, true)
			if err != nil {
				return err
			}
			entry.Object = sd
			wm.objs[objNr] = true
			return nil
		}

		if wm.objs[objNr] {
			// wm already applied to this content stream.
		} else {
			// Patch first content stream.
			err := patchFirstContentForWM(&sd)
			if err != nil {
				return err
			}
			entry.Object = sd
			wm.objs[objNr] = true
		}

		// Patch last content stream.
		o1 = o[len(o)-1]

		ir, _ = o1.(IndirectRef)
		objNr = ir.ObjectNumber.Value()
		if wm.objs[objNr] {
			// wm already applied to this content stream.
			return nil
		}

		genNr = ir.GenerationNumber.Value()
		entry, _ = xRefTable.FindTableEntry(objNr, genNr)
		sd, _ = (entry.Object).(StreamDict)

		err := patchContentForWM(&sd, gsID, xoID, wm, false)
		if err != nil {
			return err
		}

		entry.Object = sd
		wm.objs[objNr] = true
	}

	return nil
}

func viewPort(xRefTable *XRefTable, a *InheritedPageAttrs) *Rectangle {

	visibleRegion := a.mediaBox
	if a.cropBox != nil {
		visibleRegion = a.cropBox
	}

	return visibleRegion
}

func addPageWatermark(xRefTable *XRefTable, i int, wm *Watermark) error {

	log.Debug.Printf("addPageWatermark page:%d\n", i)
	if wm.Update {
		log.Debug.Println("Updating")
		if _, err := removePageWatermark(xRefTable, i); err != nil {
			return err
		}
	}

	d, inhPAttrs, err := xRefTable.PageDict(i)
	if err != nil {
		return err
	}

	wm.vp = viewPort(xRefTable, inhPAttrs)

	err = createForm(xRefTable, wm, false)
	if err != nil {
		return err
	}

	wm.pageRot = float64(inhPAttrs.rotate)

	log.Debug.Printf("\n%s\n", wm)

	gsID := "GS0"
	xoID := "Fm0"

	if inhPAttrs.resources == nil {
		err = insertPageResourcesForWM(xRefTable, d, wm, gsID, xoID)
	} else {
		err = updatePageResourcesForWM(xRefTable, inhPAttrs.resources, wm, &gsID, &xoID)
	}
	if err != nil {
		return err
	}

	obj, found := d.Find("Contents")
	if !found {
		return insertPageContentsForWM(xRefTable, d, wm, gsID, xoID)
	}

	return updatePageContentsForWM(xRefTable, obj, wm, gsID, xoID)
}

func patchContentForWM(sd *StreamDict, gsID, xoID string, wm *Watermark, saveGState bool) error {

	// Decode streamDict for supported filters only.
	err := decodeStream(sd)
	if err == filter.ErrUnsupportedFilter {
		log.Info.Println("unsupported filter: unable to patch content with watermark.")
		return nil
	}
	if err != nil {
		return err
	}

	bb := wmContent(wm, gsID, xoID)

	if wm.OnTop {
		if saveGState {
			sd.Content = append([]byte("q "), sd.Content...)
		}
		sd.Content = append(sd.Content, []byte(" Q")...)
		sd.Content = append(sd.Content, bb...)
	} else {
		sd.Content = append(bb, sd.Content...)
	}

	return encodeStream(sd)
}

func patchFirstContentForWM(sd *StreamDict) error {

	err := decodeStream(sd)
	if err == filter.ErrUnsupportedFilter {
		log.Info.Println("unsupported filter: unable to patch content with watermark.")
		return nil
	}
	if err != nil {
		return err
	}

	sd.Content = append([]byte("q "), sd.Content...)

	return encodeStream(sd)
}

// AddWatermarks adds watermarks to all pages selected.
func AddWatermarks(ctx *Context, selectedPages IntSet, wm *Watermark) error {

	log.Debug.Printf("AddWatermarks wm:\n%s\n", wm)

	xRefTable := ctx.XRefTable

	if err := prepareOCPropertiesInRoot(ctx, wm); err != nil {
		return err
	}

	if err := createResourcesForWM(ctx, wm); err != nil {
		return err
	}

	if err := createExtGStateForStamp(xRefTable, wm); err != nil {
		return err
	}

	for k, v := range selectedPages {
		if v {
			if err := addPageWatermark(xRefTable, k, wm); err != nil {
				return err
			}
		}
	}

	xRefTable.EnsureVersionForWriting()

	return nil
}

func removeResDictEntry(xRefTable *XRefTable, d *Dict, entry string, ids []string, i int) error {

	o, ok := d.Find(entry)
	if !ok {
		return errors.Errorf("page %d: corrupt resource dict", i)
	}

	d1, err := xRefTable.DereferenceDict(o)
	if err != nil {
		return err
	}

	for _, id := range ids {
		o, ok := d1.Find(id)
		if ok {
			err = xRefTable.deleteObject(o)
			if err != nil {
				return err
			}
			d1.Delete(id)
		}
	}

	if d1.Len() == 0 {
		d.Delete(entry)
	}

	return nil
}

func removeExtGStates(xRefTable *XRefTable, d *Dict, ids []string, i int) error {
	return removeResDictEntry(xRefTable, d, "ExtGState", ids, i)
}

func removeForms(xRefTable *XRefTable, d *Dict, ids []string, i int) error {
	return removeResDictEntry(xRefTable, d, "XObject", ids, i)
}

func removeArtifacts(sd *StreamDict, i int) (ok bool, extGStates []string, forms []string, err error) {

	err = decodeStream(sd)
	if err == filter.ErrUnsupportedFilter {
		log.Info.Println("unsupported filter: unable to patch content with watermark.")
		return false, nil, nil, nil
	}
	if err != nil {
		return false, nil, nil, err
	}

	var patched bool

	// Watermarks may be at the beginning or the end of the content stream.

	for {
		s := string(sd.Content)
		beg := strings.Index(s, "/Artifact <</Subtype /Watermark /Type /Pagination >>BDC")
		if beg < 0 {
			break
		}

		end := strings.Index(s[beg:], "EMC")
		if end < 0 {
			break
		}

		// Check for usage of resources.
		t := s[beg : beg+end]

		i := strings.Index(t, "/GS")
		if i > 0 {
			j := i + 3
			k := strings.Index(t[j:], " gs")
			if k > 0 {
				extGStates = append(extGStates, "GS"+t[j:j+k])
			}
		}

		i = strings.Index(t, "/Fm")
		if i > 0 {
			j := i + 3
			k := strings.Index(t[j:], " Do")
			if k > 0 {
				forms = append(forms, "Fm"+t[j:j+k])
			}
		}

		// TODO Remove whitespace until 0x0a
		sd.Content = append(sd.Content[:beg], sd.Content[beg+end+3:]...)
		patched = true
	}

	if patched {
		err = encodeStream(sd)
	}

	return patched, extGStates, forms, err
}

func removeArtifactsFromPage(xRefTable *XRefTable, sd *StreamDict, resDict *Dict, i int) (bool, error) {

	// Remove watermark artifacts and locate id's
	// of used extGStates and forms.
	ok, extGStates, forms, err := removeArtifacts(sd, i)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	// Remove obsolete extGStates from page resource dict.
	err = removeExtGStates(xRefTable, resDict, extGStates, i)
	if err != nil {
		return false, err
	}

	// Remove obsolete extGStatesforms from page resource dict.
	return true, removeForms(xRefTable, resDict, forms, i)
}

func locatePageContentAndResourceDict(xRefTable *XRefTable, i int) (Object, Dict, error) {

	d, _, err := xRefTable.PageDict(i)
	if err != nil {
		return nil, nil, err
	}

	o, found := d.Find("Resources")
	if !found {
		return nil, nil, errors.Errorf("page %d: no resource dict found\n", i)
	}

	resDict, err := xRefTable.DereferenceDict(o)
	if err != nil {
		return nil, nil, err
	}

	o, found = d.Find("Contents")
	if !found {
		return nil, nil, errors.Errorf("page %d: no page watermark found", i)
	}

	return o, resDict, nil
}

func removePageWatermark(xRefTable *XRefTable, i int) (bool, error) {

	o, resDict, err := locatePageContentAndResourceDict(xRefTable, i)
	if err != nil {
		return false, err
	}

	found := false
	var entry *XRefTableEntry

	ir, ok := o.(IndirectRef)
	if ok {
		objNr := ir.ObjectNumber.Value()
		genNr := ir.GenerationNumber.Value()
		entry, _ = xRefTable.FindTableEntry(objNr, genNr)
		o = entry.Object
	}

	switch o := o.(type) {

	case StreamDict:
		ok, err := removeArtifactsFromPage(xRefTable, &o, &resDict, i)
		if err != nil {
			return false, err
		}
		if !found && ok {
			found = true
		}
		entry.Object = o

	case Array:
		// Get stream dict for first element.
		o1 := o[0]
		ir, _ := o1.(IndirectRef)
		objNr := ir.ObjectNumber.Value()
		genNr := ir.GenerationNumber.Value()
		entry, _ := xRefTable.FindTableEntry(objNr, genNr)
		sd, _ := (entry.Object).(StreamDict)

		ok, err := removeArtifactsFromPage(xRefTable, &sd, &resDict, i)
		if err != nil {
			return false, err
		}
		if !found && ok {
			found = true
			entry.Object = sd
		}

		if len(o) > 1 {
			// Get stream dict for last element.
			o1 := o[len(o)-1]
			ir, _ := o1.(IndirectRef)
			objNr = ir.ObjectNumber.Value()
			genNr := ir.GenerationNumber.Value()
			entry, _ := xRefTable.FindTableEntry(objNr, genNr)
			sd, _ := (entry.Object).(StreamDict)

			ok, err = removeArtifactsFromPage(xRefTable, &sd, &resDict, i)
			if err != nil {
				return false, err
			}
			if !found && ok {
				found = true
				entry.Object = sd
			}
		}

	}

	/*
		Supposedly the form needs a PieceInfo in order to be recognized by Acrobat like so:

			<PieceInfo, <<
				<ADBE_CompoundType, <<
					<DocSettings, (61 0 R)>
					<LastModified, (D:20190830152436+02'00')>
					<Private, Watermark>
				>>>
			>>>

	*/

	return found, nil
}

func locateOCGs(ctx *Context) (Array, error) {

	rootDict, err := ctx.Catalog()
	if err != nil {
		return nil, err
	}

	o, ok := rootDict.Find("OCProperties")
	if !ok {
		return nil, errNoWatermark
	}

	d, err := ctx.DereferenceDict(o)
	if err != nil {
		return nil, err
	}

	o, found := d.Find("OCGs")
	if !found {
		return nil, errNoWatermark
	}

	return ctx.DereferenceArray(o)
}

// RemoveWatermarks removes watermarks for all pages selected.
func RemoveWatermarks(ctx *Context, selectedPages IntSet) error {

	log.Debug.Printf("RemoveWatermarks\n")

	a, err := locateOCGs(ctx)
	if err != nil {
		return err
	}

	found := false

	for _, o := range a {
		d, err := ctx.DereferenceDict(o)
		if err != nil {
			return err
		}

		if o == nil {
			continue
		}

		if *d.Type() != "OCG" {
			continue
		}

		n := d.StringEntry("Name")
		if n == nil {
			continue
		}

		if *n != "Background" && *n != "Watermark" {
			continue
		}

		found = true
		break
	}

	if !found {
		return errNoWatermark
	}

	var removedSmth bool

	for k, v := range selectedPages {
		if !v {
			continue
		}

		ok, err := removePageWatermark(ctx.XRefTable, k)
		if err != nil {
			return err
		}

		if ok {
			removedSmth = true
		}
	}

	if !removedSmth {
		return errNoWatermark
	}

	return nil
}
