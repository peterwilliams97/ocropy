package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image" // for color.Alpha{a}
	"image/draw"
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

	rgba := overlay(img, rects)

	err = save(outPath, rgba)
	if err != nil {
		panic(err)
	}
	fmt.Printf("saved to %q\n", outPath)

	err = saveRectList(maskPath, rectList)
	if err != nil {
		panic(err)
	}

}

func overlay(img image.Image, rectList []Rect) image.Image {
	bounds := img.Bounds()
	fmt.Printf("bounds=%#v\n", bounds)
	w, h := bounds.Max.X, bounds.Max.Y
	fmt.Printf("w=%d h=%d\n", w, h)

	rgba := image.NewRGBA(img.Bounds())

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rgba.Set(x, y, image.Transparent)
		}
	}
	// rgba := image.NewRGBA(img.Bounds())
	// draw.Draw(rgba, rgba.Bounds(), img, image.Point{0, 0}, draw.Src)
	for _, r := range rectList {
		draw.Draw(rgba, r.bounds(), img, r.position(), draw.Src)
	}

	// for y := 0; y < h/2; y++ {
	// 	for x := 0; x < w/2; x++ {
	// 		rgba.Set(x, y, image.Transparent)
	// 	}
	// }
	return rgba
}

var rectList = []Rect{
	Rect{50, 50, 450, 650},
	Rect{500, 1000, 900, 1800},
}

type Rect struct {
	X0, Y0, X1, Y1 int
}

func (r Rect) bounds() image.Rectangle {
	return image.Rect(r.X0, r.Y0, r.X1, r.Y1)
}

func (r Rect) position() image.Point {
	return image.Point{r.X0, r.Y0}
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

func save(filename string, img image.Image) error {
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	return png.Encode(out, img)
}

// makeUsage updates flag.Usage to include usage message `msg`.
func makeUsage(msg string) {
	usage := flag.Usage
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, msg)
		usage()
	}
}
