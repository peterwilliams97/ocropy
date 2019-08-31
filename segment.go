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
	origPathPng := changeExt(outPath, ".orig.png")
	origPathJpg := changeExt(outPath, ".orig.jpg")

	rectList, err := loadRectList(maskPath)
	if err != nil {
		panic(err)
	}
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

	dilation := 50
	// fgdList := makeForegroundList(img, dilate(rectList, dilation))
	fgdList := makeForegroundList(img, rectList)
	bgd := makeBackground(img, dilate(rectList, -dilation))

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
		fgdPath := makeFgdPath(outPath, i)
		err = saveImage(fgdPath, fgd, true)
		if err != nil {
			panic(err)
		}
		fmt.Printf("saved foreground to %q\n", fgdPath)
		fgdPathList = append(fgdPathList, fgdPath)
	}

	err = saveImage(bgdPath, bgd, false)
	if err != nil {
		panic(err)
	}
	fmt.Printf("saved background to %q\n", bgdPath)

	overlayImages(bgdPath, rectList, fgdPathList, outPath, w, h, dilation)
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
func overlayImages(bgdPath string, rectList []Rect, fgdPathList []string, outPath string,
	w, h, dilation int) error {
	scale, xOfs, yOfs := computeScale(width, height, float64(w), float64(h))
	common.Log.Info("overlayImages: scale=%.3f width=%.1f height=%.1f w=%d h=%d",
		scale, width, height, w, h)
	common.Log.Info("               scale * w x h = %.1f x%.1f", scale*float64(w), scale*float64(h))
	c := creator.New()
	c.NewPage()

	r := Rect{X0: 0, Y0: 0, X1: w, Y1: h}
	dctEnc := core.NewDCTEncoder()
	dctEnc.Width = w
	dctEnc.Height = h
	if err := addImage(c, bgdPath, dctEnc, r, scale, xOfs, yOfs, 0); err != nil { // !@#$ DCT
		return err
	}
	for i, fgdPath := range fgdPathList {
		r := rectList[i]
		if err := addImage(c, fgdPath, core.NewFlateEncoder(), r, scale, xOfs, yOfs, dilation); err != nil {
			return err
		}
	}

	return c.WriteToFile(outPath)
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
	// common.Log.Info("addImage: widthMM=%.2f heightMM=%.2f", widthMM, heightMM)
	// common.Log.Info("addImage: widthPt=%.2f heightPt=%.2f", widthPt, heightPt)

	// x, y := float64(r.X0+dilation)*scale+xOfs, float64(r.Y0-dilation)*scale+yOfs
	// x, y := float64(r.X0-dilation)*scale, float64(r.Y0)*scale
	// x, y := float64(r.X0-dilation)*scale, float64(r.Y0)*scale
	x, y := float64(r.X0)*scale+xOfs, float64(r.Y0)*scale+yOfs

	w, h := float64(r.X1-r.X0)*scale, float64(r.Y1-r.Y0)*scale // +1? !@#$
	common.Log.Info("addImage: r=%v scale=%.3f xOfs=%.3f yOfs=%.3f", r, scale, xOfs, yOfs)
	common.Log.Info("addImage: xPos=%6.2f yPos=%6.2f width=%6.2f height=%6.2f %q", x, y, w, h, imgPath)
	// img.Scale(scale, scale)
	// img.SetPos(x-float64(dilation), y+float64(dilation))
	img.SetPos(x, y)
	img.SetWidth(w)
	img.SetHeight(h)
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

const imageDir = "images"

func makeFgdPath(outPath string, i int) string {
	return changeExt(outPath, fmt.Sprintf("%03d.fgd.png", i))
}

func changeExt(filename, newExt string) string {
	base := filepath.Base(filename)
	filename = filepath.Join(imageDir, base)
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
