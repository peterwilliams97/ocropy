#!/usr/bin/env python

from PIL import Image, ImageDraw
import random as pyrandom
import sys
import os.path
import re
import glob
import argparse
import codecs

import numpy as np
from matplotlib.pyplot import imread

import ocrolib
from ocrolib import hocr, common

"""
Construct an HTML output file in hOCR format by putting together
the recognition results for each page in sequence.
You should usually invoke this program as

    ocropus-hocr 'book/????.bin.png'

For each page like 'book/0001.bin.png', it uses the following files:

    book/0001.bin.png            # page image
    book/0001.pseg.png           # page segmentation
    book/0001/010001.txt         # recognizer output for lines

      # perform binarization
    ./ocropus-nlbin tests/ersch.png -o book

    # perform page layout analysis
    ./ocropus-gpageseg 'book/????.bin.png'
"""

desc = common.desc


parser = argparse.ArgumentParser()
parser.add_argument("-e","--nobreaks",action="store_true",help="don't output line breaks")
parser.add_argument("-p","--nopars",action="store_true",help="don't output paragraphs")
parser.add_argument("-s","--fscale",type=float,default=1.0,help="scale factor for translating xheights into font size (use 0 to disable), default: %(default)s")
parser.add_argument("-i", "--input", default="", help="input file")
parser.add_argument("-b", "--bin", default="", help="bin file")

args = parser.parse_args()
inFile = args.input
binFile = args.bin
inBase, _ = ocrolib.allsplitext(inFile)
binBase, _ = ocrolib.allsplitext(binFile)
print("**  inBase=%s" % inBase)
print("** binBase=%s" % binBase)

outFile = inBase + ".out.png"

print(" inFile=%s" % inFile)
print("binFile=%s" % binFile)
print("outFile=%s" % outFile)
assert inFile and binFile
assert outFile != inFile
assert outFile != binFile


image = ocrolib.read_image_binary(binFile)
print("$$ %s=%s" % (binFile, desc(image)))
height, width = image.shape

# to proceed, we need a pseg file and a subdirectory containing text lines
psegPath = binBase + ".pseg.png"
assert os.path.exists(psegPath), "%s: no such file" % psegPath
assert os.path.isdir(binBase), "%s: no such directory" % binBase

# iterate through the text lines in reading order, based on the page segmentation file
pseg = ocrolib.read_page_segmentation(psegPath)
print("$$ %s=%s" % (psegPath, desc(pseg)))

regions = ocrolib.RegionExtractor()
print("$$ regions=%s" % regions)
regions.setPageLines(pseg)


im = Image.open(inFile)
print("%s %s" % (inFile, im.size))

for i in range(1, regions.length()):

    id = regions.id(i)
    y0, x0, y1, x1 = regions.bbox(i)
    print("%5d: 0x%05X %s %d x %d" % (i, id, [y0, x0, y1, x1], y1 - y0, x1 - x0))

    draw = ImageDraw.Draw(im)
    draw.rectangle((x0, y0, x1, y1), outline=(255, 0, 0), width=3)
    draw.rectangle((x0, y0, x1, y1), outline=(0, 0, 255), width=0)
    # draw.rectangle((x0, y0, x1, y1), outline=255, width=5)
    # draw.rectangle((x0, y0, x1, y1), outline=10,  width=1)
    del draw


# write to stdout
im.save(outFile, "PNG")
