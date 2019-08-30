package main

import (
	"flag"
	"fmt"
	"image" // for color.Alpha{a}
	"image/png"
	"os"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/creator"
)

func init() {
	// without this register .. At(), Bounds() functions will
	// caused memory pointer error!!
	image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)
}

const usage = "Make masked image"

func main() {
	common.SetLogger(common.NewConsoleLogger(common.LogLevelInfo))

	var bgdPath, fgdPath, outPath string
	flag.StringVar(&bgdPath, "b", "", "Background image.")
	flag.StringVar(&fgdPath, "f", "", "Foregorund image.")
	flag.StringVar(&outPath, "o", "", "Output PDF.")
	makeUsage(usage)
	flag.Parse()
	if bgdPath == "" || fgdPath == "" || outPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	err := overlay(bgdPath, fgdPath, outPath)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Wrote %q\n", outPath)
}

const (
	width  = 210.0 / 25.4 * 72.0
	height = 297.0 / 25.4 * 72.0
	xPos   = 0.0
	yPos   = 0.0
)

// Add image to a specific page of a PDF.  xPos and yPos define the upper left corner of the image location, and iwidth
// is the width of the image in PDF document dimensions (height/width ratio is maintained).
func overlay(bgdPath, fgdPath, outPath string) error {

	c := creator.New()

	// Prepare the images.
	fimg, err := c.NewImageFromFile(fgdPath)
	if err != nil {
		return err
	}
	fimg.ScaleToWidth(width)
	fimg.SetPos(xPos, yPos)

	bimg, err := c.NewImageFromFile(bgdPath)
	if err != nil {
		return err
	}
	bimg.ScaleToWidth(width)
	bimg.SetPos(xPos, yPos)

	c.NewPage()
	err = c.Draw(bimg)
	if err != nil {
		return err
	}
	err = c.Draw(fimg)
	if err != nil {
		return err
	}

	err = c.WriteToFile(outPath)
	return err
}

// makeUsage updates flag.Usage to include usage message `msg`.
func makeUsage(msg string) {
	usage := flag.Usage
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, msg)
		usage()
	}
}
