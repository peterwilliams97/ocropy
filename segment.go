package main

/*

<</Filter /FlateDecode/Type /XObject/Subtype /Image
 /Width 2479/Height 3508/BitsPerComponent 8
 /ColorSpace /DeviceRGB
 /Length 2,681,392>>

 <</Filter /FlateDecode/Type /XObject/Subtype /Image
  /Width 2479/Height 3508/BitsPerComponent 8
  /ColorSpace /DeviceRGB
  /SMask 9 0 R
  /Length 1,915,183>>

*/

import (
	"encoding/json"
	"flag"
	"fmt"
	"image" // for color.Alpha{a}
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/core"
	"github.com/unidoc/unipdf/v3/creator"
)

const (
	fgdIsPng    = false
	bgdIsPng    = true
	jpegQuality = 25
)

const usage = "Make masked image"

func main() {
	common.SetLogger(common.NewConsoleLogger(common.LogLevelInfo))
	makeUsage(usage)
	flag.Parse()
	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}
	if _, err := os.Stat(imageDir); os.IsNotExist(err) {
		os.Mkdir(imageDir, 0777)
	}
	for _, inPath := range flag.Args() {
		err := processDoc(inPath, false, true, true)
		if err != nil {
			panic(err)
		}
		err = processDoc(inPath, true, false, false)
		if err != nil {
			panic(err)
		}
		err = processDoc(inPath, true, false, true)
		if err != nil {
			panic(err)
		}
		err = processDoc(inPath, false, false, false)
		if err != nil {
			panic(err)
		}
	}
}

func processDoc(inPath string, simple, bgdOnly, isPng bool) error {
	var outPath string
	if simple {
		names := map[bool]string{false: "jpg", true: "png"}
		outPath = changeExtOnly(inPath, fmt.Sprintf(".unmasked.%s.pdf", names[isPng]))
	} else if bgdOnly {
		outPath = changeExtOnly(inPath, ".bgd.pdf")
	} else {
		outPath = changeExtOnly(inPath, ".masked.pdf")
	}
	pageRectList, err := loadPageRectList(inPath)
	if err != nil {
		return err
	}
	common.Log.Info("processDoc: %d pages\n\t   %q\n\t-> %q", len(pageRectList), inPath, outPath)

	pagePaths := pageKeys(pageRectList)

	c := creator.New()
	for _, pagePath := range pagePaths {
		rectList := pageRectList[pagePath]
		err := addImageToPage(c, pagePath, rectList, simple, bgdOnly, isPng)
		if err != nil {
			return err
		}
	}
	err = c.WriteToFile(outPath)
	if err != nil {
		return err
	}
	common.Log.Info("processDoc: %d pages\n\t   %q\n\t-> %q", len(pageRectList), inPath, outPath)

	return nil
}

func addImageToPage(c *creator.Creator, inPath string, rectList []Rect,
	simple, bgdOnly, isPng bool) error {
	bgdPath := changeExt(inPath, ".bgd.png")
	origPathPng := changeExt(inPath, ".orig.png")
	origPathJpg := changeExt(inPath, ".orig.jpg")

	fmt.Printf("rectList=%#v\n", rectList)

	imgfile, err := os.Open(inPath)
	if err != nil {
		fmt.Printf("%q file not found!\n", inPath)
		os.Exit(1)
	}
	defer imgfile.Close()

	img, _, err := image.Decode(imgfile)
	if err != nil {
		panic(err)
	}
	bounds := img.Bounds()
	w, h := bounds.Max.X, bounds.Max.Y

	if simple {
		return placeImageOnPage(c, inPath, w, h, isPng)
	}

	dilation := 2
	fgdList := makeForegroundList(img, rectList)
	bgd := makeBackground(img, dilate(rectList, -dilation), bgdOnly)

	err = saveImage(origPathPng, img, true)
	if err != nil {
		panic(err)
	}
	fmt.Printf("saved original to %q\n", origPathPng)

	err = saveImage(origPathJpg, img, false)
	if err != nil {
		panic(err)
	}
	fmt.Printf("saved original to %q\n", origPathJpg)

	var fgdPathList []string
	for i, fgd := range fgdList {
		fgdPath := makeFgdPath(inPath, i)
		err = saveImage(fgdPath, fgd, fgdIsPng)
		if err != nil {
			panic(err)
		}
		fmt.Printf("saved foreground to %q\n", fgdPath)
		fgdPathList = append(fgdPathList, fgdPath)
	}

	err = saveImage(bgdPath, bgd, bgdIsPng)
	if err != nil {
		panic(err)
	}
	fmt.Printf("saved background to %q\n", bgdPath)

	if bgdOnly {
		return placeImageOnPage(c, bgdPath, w, h, isPng)
	}

	err = overlayImagesOnPage(c, bgdPath, rectList, fgdPathList, w, h, dilation)
	if err != nil {
		panic(err)
	}
	return nil
}

const (
	widthMM    = 210.0
	heightMM   = 297.0
	widthInch  = 8.5
	heightInch = 11.0
	// widthPt  = (widthMM / 25.4) * 72.0
	// heightPt = (heightMM / 25.4) * 72.0
	widthPt  = widthInch * 72.0
	heightPt = heightInch * 72.0
	// xPos     = widthPt / 10.0
	// yPos     = heightPt / 10.0
	xPos   = 0.0
	yPos   = 0.0
	width  = widthPt - 2*xPos
	height = heightPt - 2*yPos
)

// computeScale returns the scale for w x h -> width x height
func computeScale(width, height, w, h float64) (scale, xOfs, yOfs float64) {
	xScale := width / w
	yScale := height / h
	if xScale < yScale {
		scale = xScale
		yOfs = 0.5 * (height - scale*h)
	} else {
		scale = yScale
		xOfs = 0.5 * (width - scale*w)
	}
	if xOfs < 0 || yOfs < 0 {
		panic("Can't happend")
	}
	return
}

// overlay image in `fgdPath` over image in `bgdPath` (currently assumed to be have the same
// dimensions `w` x `h`) and write the resulting single page `width` x `height` PDF to `outPath`.
// is the width of the image in PDF document dimensions (height/width ratio is maintained).
func overlayImagesOnPage(c *creator.Creator, bgdPath string, rectList []Rect, fgdPathList []string,
	w, h, dilation int) error {
	scale, xOfs, yOfs := computeScale(width, height, float64(w), float64(h))
	common.Log.Info("overlayImagesOnPage: scale=%.3f width=%.1f height=%.1f w=%d h=%d",
		scale, width, height, w, h)
	common.Log.Info("               scale * w x h = %.1f x%.1f", scale*float64(w), scale*float64(h))
	// c := creator.New()
	c.NewPage()

	r := Rect{X0: 0, Y0: 0, X1: w, Y1: h}
	enc := makeEncoder(bgdIsPng, w, h)
	if err := addImage(c, bgdPath, enc, r, scale, xOfs, yOfs, 0); err != nil {
		return err
	}
	for i, fgdPath := range fgdPathList {
		r := rectList[i]
		enc := makeEncoder(fgdIsPng, w, h)
		if err := addImage(c, fgdPath, enc, r, scale, xOfs, yOfs, dilation); err != nil {
			return err
		}
	}
	return nil
}

func placeImageOnPage(c *creator.Creator, bgdPath string, w, h int, isPng bool) error {
	scale, xOfs, yOfs := computeScale(width, height, float64(w), float64(h))
	common.Log.Info("placeImageOnPage: scale=%.3f width=%.1f height=%.1f w=%d h=%d",
		scale, width, height, w, h)
	common.Log.Info("               scale * w x h = %.1f x%.1f", scale*float64(w), scale*float64(h))
	c.NewPage()

	r := Rect{X0: 0, Y0: 0, X1: w, Y1: h}
	enc := makeEncoder(isPng, w, h)
	if err := addImage(c, bgdPath, enc, r, scale, xOfs, yOfs, 0); err != nil {
		return err
	}
	return nil
}

func makeEncoder(isPng bool, w, h int) core.StreamEncoder {
	if isPng {
		return core.NewFlateEncoder()
	}
	dctEnc := core.NewDCTEncoder()
	dctEnc.Width = w
	dctEnc.Height = h
	dctEnc.Quality = jpegQuality
	return dctEnc
}

// addImage adds image in `imagePath` to `c` with encoding and scale given by `encoder` and `scale`.
func addImage(c *creator.Creator, imgPath string, encoder core.StreamEncoder,
	r Rect, scale, xOfs, yOfs float64, dilation int) error {
	common.Log.Info("==================================================== addImage")
	img, err := c.NewImageFromFile(imgPath)
	if err != nil {
		return err
	}
	if encoder != nil {
		img.SetEncoder(encoder)
	}
	x, y := float64(r.X0)*scale+xOfs, float64(r.Y0)*scale+yOfs
	w, h := float64(r.X1-r.X0)*scale, float64(r.Y1-r.Y0)*scale // +1? !@#$
	common.Log.Info("addImage: r=%v scale=%.3f xOfs=%.3f yOfs=%.3f", r, scale, xOfs, yOfs)
	common.Log.Info("addImage: xPos=%6.2f yPos=%6.2f width=%6.2f height=%6.2f %q", x, y, w, h, imgPath)
	img.SetPos(x, y)
	img.SetWidth(w)
	img.SetHeight(h)
	return c.Draw(img)
}

// makeForeground returns `img` masked to the rectangles in `rectList`.
func _makeForeground(img image.Image, rectList []Rect) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Max.X, bounds.Max.Y
	r := fromBounds(bounds)
	fmt.Printf("makeForeground: rectList=%v\n", rectList)
	fmt.Printf("bounds=%#v\n", bounds)
	fmt.Printf("r=%#v\n", r)
	fmt.Printf("w=%d h=%d\n", w, h)

	rgba := image.NewRGBA(img.Bounds())
	fillRect(rgba, r, image.Transparent)
	for _, r := range rectList {
		draw.Draw(rgba, r.bounds(), img, r.position(), draw.Src)
		// fillRect(rgba, r, image.White)
	}
	return rgba
}

// makeForegroundList images of returns `img` clipped by the rectangles in `rectList`.
func makeForegroundList(img image.Image, rectList []Rect) []image.Image {
	bounds := img.Bounds()
	w, h := bounds.Max.X, bounds.Max.Y
	r := fromBounds(bounds)
	fmt.Printf("makeForegroundList: rectList=%v\n", rectList)
	fmt.Printf("bounds=%#v\n", bounds)
	fmt.Printf("r=%#v\n", r)
	fmt.Printf("w=%d h=%d\n", w, h)

	fgdList := make([]image.Image, len(rectList))
	for i, r := range rectList {
		rgba := image.NewRGBA(r.zpBounds())
		wind := r.bounds()
		draw.Draw(rgba, r.zpBounds(), img, r.position(), draw.Src)
		fgdList[i] = rgba
		fmt.Printf("%4d: %v=%v -> %v\n", i, r, wind, rgba.Bounds())
	}
	return fgdList
}

// makeForeground returns `img` not masked to the rectangles in `rectList`.
func makeBackground(img image.Image, rectList []Rect, blackBgd bool) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Max.X, bounds.Max.Y
	r := fromBounds(bounds)
	fmt.Printf("makeBackground: rectList=%v\n", rectList)
	fmt.Printf("bounds=%#v\n", bounds)
	fmt.Printf("r=%#v\n", r)
	fmt.Printf("w=%d h=%d\n", w, h)

	bgdColor := image.White
	if blackBgd {
		bgdColor = image.NewUniform(color.RGBA{B: 0xFF, A: 0xFF})
		fmt.Printf("@@ bgdColor=%#v\n", bgdColor)
	}

	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, r.bounds(), img, r.position(), draw.Src)
	for _, r := range rectList {
		fillRect(rgba, r, bgdColor)
	}
	return rgba
}

type Rect struct {
	X0, Y0, X1, Y1 int
}

func fromBounds(b image.Rectangle) Rect {
	return Rect{
		X0: b.Min.X,
		Y0: b.Min.Y,
		X1: b.Max.X,
		Y1: b.Max.Y,
	}
}

func (r Rect) bounds() image.Rectangle {
	return image.Rect(r.X0, r.Y0, r.X1, r.Y1)
}

func (r Rect) position() image.Point {
	return image.Point{r.X0, r.Y0}
}

func (r Rect) zpBounds() image.Rectangle {
	return image.Rect(0, 0, r.X1-r.X0, r.Y1-r.Y0)
}

// dilate returns the Rects in `rectList` dilated by `d` on all 4 sides
func dilate(rectList []Rect, d int) []Rect {
	outList := make([]Rect, len(rectList))
	for i, r := range rectList {
		if r.X1-r.X0 < 2*d || r.Y1-r.Y0 < 2*d {
			common.Log.Error("r=%+v dilation=%d", r, d)
			panic("not allowed")
		}
		r.X0 -= d
		r.X1 += d
		r.Y0 -= d
		r.Y1 += d
		outList[i] = r
	}
	fmt.Printf("dilate: d=%d %v->%v\n", d, rectList, outList)
	return outList
}

func fillRect(img *image.RGBA, r Rect, col *image.Uniform) {
	for y := r.Y0; y < r.Y1; y++ {
		for x := r.X0; x < r.X1; x++ {
			img.Set(x, y, col)
		}
	}
}

func saveRectList(filename string, rectList []Rect) error {
	b, err := json.MarshalIndent(rectList, "", "\t")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filename, b, 0644)
	return err
}

func loadPageRectList(filename string) (map[string][]Rect, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var pageRectList map[string][]Rect
	err = json.Unmarshal(b, &pageRectList)
	return pageRectList, err
}

func pageKeys(pageRectList map[string][]Rect) []string {
	keys := make([]string, 0, len(pageRectList))
	for page := range pageRectList {
		keys = append(keys, page)
	}
	sort.Strings(keys)
	return keys
}

func drawPixels(img *image.Alpha, px, py, pw, ph uint, fill bool) {
	var x, y uint
	for y = 0; y < ph; y++ {
		for x = 0; x < pw; x++ {
			if fill {
				img.Set(int(px*pw+x), int(py*ph+y), image.White)
			} else {
				img.Set(int(px*pw+x), int(py*ph+y), image.Transparent)
			}
		}
	}
}

func saveImage(filename string, img image.Image, isPng bool) error {
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	if isPng {
		return png.Encode(out, img)
	}
	return jpeg.Encode(out, img, nil)
}

const imageDir = "images"

func makeFgdPath(outPath string, i int) string {
	return changeExt(outPath, fmt.Sprintf("-%03d.fgd.png", i))
}

func changeExt(filename, newExt string) string {
	base := filepath.Base(filename)
	filename = filepath.Join(imageDir, base)
	return changeExtOnly(filename, newExt)
}

func changeExtOnly(filename, newExt string) string {
	ext := filepath.Ext(filename)
	return filename[:len(filename)-len(ext)] + newExt
}

// makeUsage updates flag.Usage to include usage message `msg`.
func makeUsage(msg string) {
	usage := flag.Usage
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, msg)
		usage()
	}
}
