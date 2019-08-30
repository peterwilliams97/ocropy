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
	"image/draw"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/core"
	"github.com/unidoc/unipdf/v3/creator"
)

func init() {
	// without this register .. At(), Bounds() functions will
	// caused memory pointer error!!
	// image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)
}

const usage = "Make masked image"

func main() {
	common.SetLogger(common.NewConsoleLogger(common.LogLevelInfo))

	var inPath, maskPath, outPath string
	flag.StringVar(&inPath, "i", "", "Input image.")
	flag.StringVar(&maskPath, "m", "", "JSON file containing rectangles of images.")
	flag.StringVar(&outPath, "o", "", "Output PDF files.")
	makeUsage(usage)
	flag.Parse()
	if inPath == "" || maskPath == "" || outPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	bgdPath := changeExt(outPath, ".bgd.jpg")
	fgdPath := changeExt(outPath, ".fgd.png")
	origPathPng := changeExt(outPath, ".orig.png")
	origPathJpg := changeExt(outPath, ".orig.jpg")

	rects, err := loadRectList(maskPath)
	if err != nil {
		panic(err)
	}
	fmt.Printf("rects=%#v\n", rects)

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

	dilation := 10
	fgd := makeForeground(img, dilate(rects, dilation))
	bgd := makeBackground(img, dilate(rects, -dilation))

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

	err = saveImage(fgdPath, fgd, true)
	if err != nil {
		panic(err)
	}
	fmt.Printf("saved foreground to %q\n", fgdPath)

	err = saveImage(bgdPath, bgd, false)
	if err != nil {
		panic(err)
	}
	fmt.Printf("saved background to %q\n", bgdPath)

	overlayImages(bgdPath, fgdPath, outPath, w, h)
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
func computeScale(width, height, w, h float64) float64 {
	xScale := width / w
	yScale := height / h
	if xScale < yScale {
		return xScale
	}
	return yScale
}

// overlay image in `fgdPath` over image in `bgdPath` (currently assumed to be have the same
// dimensions `w` x `h`) and write the resulting single page `width` x `height` PDF to `outPath`.
// is the width of the image in PDF document dimensions (height/width ratio is maintained).
func overlayImages(bgdPath, fgdPath, outPath string, w, h int) error {
	scale := computeScale(width, height, float64(w), float64(h))
	c := creator.New()
	c.NewPage()

	if err := addImage(c, bgdPath, core.NewFlateEncoder(), scale); err != nil { // !@#$ DCT
		return err
	}
	if err := addImage(c, fgdPath, core.NewFlateEncoder(), scale); err != nil {
		return err
	}

	return c.WriteToFile(outPath)
}

// addImage adds image in `imagePath` to `c` with encoding and scale given by `encoder` and `scale`.
func addImage(c *creator.Creator, imgPath string, encoder core.StreamEncoder, scale float64) error {
	img, err := c.NewImageFromFile(imgPath)
	if err != nil {
		return err
	}
	if encoder != nil {
		img.SetEncoder(encoder)
	}
	common.Log.Info("addImage: widthMM=%.2f heightMM=%.2f", widthMM, heightMM)
	common.Log.Info("addImage: widthPt=%.2f heightPt=%.2f", widthPt, heightPt)
	common.Log.Info("addImage: xPos=%.2f yPos=%.2f width=%.2f height=%.2f", xPos, yPos, width, height)
	img.Scale(scale, scale)
	img.SetPos(xPos, yPos)
	return c.Draw(img)
}

// makeForeground returns `img` masked to the rectangles in `rectList`.
func makeForeground(img image.Image, rectList []Rect) image.Image {
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

	// fgdList := make([]*image.RGBA, len(rectList))
	fgdList := make([]image.Image, len(rectList))
	for i, r := range rectList {
		rgba := image.NewRGBA(r.zpBounds())
		draw.Draw(rgba, r.zpBounds(), img, image.ZP, draw.Src)
		fgdList[i] = rgba
		fmt.Printf("%4d: %v -> %v\n", i, r, rgba.Bounds())
	}
	return fgdList
}

// makeForeground returns `img` not masked to the rectangles in `rectList`.
func makeBackground(img image.Image, rectList []Rect) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Max.X, bounds.Max.Y
	r := fromBounds(bounds)
	fmt.Printf("makeBackground: rectList=%v\n", rectList)
	fmt.Printf("bounds=%#v\n", bounds)
	fmt.Printf("r=%#v\n", r)
	fmt.Printf("w=%d h=%d\n", w, h)

	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, r.bounds(), img, r.position(), draw.Src)
	for _, r := range rectList {
		fillRect(rgba, r, image.Black)
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
		if r.X1-r.X0 > 2*d {
			r.X0 -= d
			r.X1 += d
		}
		if r.Y1-r.Y0 > 2*d {
			r.Y0 -= d
			r.Y1 += d
		}
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

func loadRectList(filename string) ([]Rect, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var rectList []Rect
	err = json.Unmarshal(b, &rectList)
	return rectList, err
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

func changeExt(filename, newExt string) string {
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
