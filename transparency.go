package main

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
)

func init() {
	// without this register .. At(), Bounds() functions will
	// caused memory pointer error!!
	image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)
}

const usage = "Make masked image"

func main() {
	var inPath, maskPath, outPath string
	flag.StringVar(&inPath, "i", "", "Input image.")
	flag.StringVar(&maskPath, "m", "", "JSON file containing rectangles of PNG.")
	flag.StringVar(&outPath, "o", "", "Output image.")
	makeUsage(usage)
	flag.Parse()
	if inPath == "" || maskPath == "" || outPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	bgdPath := outPath + ".bgd.jpg"
	fgdPath := outPath + ".fgd.png"

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

	dilation := 5
	fgd := makeForeground(img, dilate(rects, dilation))
	bgd := makeBackground(img, dilate(rects, -dilation))

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

	// err = saveRectList(maskPath, rectList)
	// if err != nil {
	// 	panic(err)
	// }

}

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
	}
	return rgba
}

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
		fillRect(rgba, r, image.White)
	}
	return rgba
}

var rectList = []Rect{
	Rect{50, 50, 450, 650},
	Rect{500, 1000, 900, 1800},
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

// makeUsage updates flag.Usage to include usage message `msg`.
func makeUsage(msg string) {
	usage := flag.Usage
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, msg)
		usage()
	}
}
