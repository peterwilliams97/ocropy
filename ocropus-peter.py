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
# from matplotlib.pyplot import imread
from scipy.ndimage import filters, interpolation, morphology, measurements
from scipy import stats

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


def check_page(image):
    if len(image.shape) == 3:
        return "input image is color image %s" % (image.shape,)
    if np.mean(image) < np.median(image):
        return "image may be inverted"
    h, w = image.shape
    if h < 600:
        return "image not tall enough for a page image %s" % (image.shape,)
    if h > 10000:
        return "image too tall for a page image %s" % (image.shape,)
    if w < 600:
        return "image too narrow for a page image %s" % (image.shape,)
    if w > 10000:
        return "line too wide for a page image %s" % (image.shape,)
    return None


def normalize_raw_image(raw):
    """ perform image normalization """
    image = raw - np.amin(raw)
    if np.amax(image) == np.amin(image):
        print("# image is empty: %s" % (fname))
        return None
    image /= np.amax(image)
    return image


def estimate_local_whitelevel(image, zoom=0.5, perc=80, size=20):
    """flatten it by estimating the local whitelevel
        zoom for page background estimation,smaller=faster
        percentage for filters
        size for filters
    """
    m = interpolation.zoom(image, zoom)
    m = filters.percentile_filter(m, perc, size=(size, 2))
    m = filters.percentile_filter(m, perc, size=(2, size))
    m = interpolation.zoom(m, 1.0/zoom)
    w, h = np.minimum(np.array(image.shape), np.array(m.shape))
    flat = np.clip(image[:w, :h] - m[:w, :h] + 1, 0, 1)
    return flat


def estimate_skew(flat, bignore=0.1, maxskew=2, skewsteps=8):
    """estimate skew angle and rotate"""
    d0, d1 = flat.shape
    o0, o1 = int(bignore*d0), int(bignore*d1)  # border ignore
    flat = np.amax(flat)-flat
    flat -= np.amin(flat)
    est = flat[o0:d0-o0, o1:d1-o1]
    ma = maxskew
    ms = int(2*maxskew*skewsteps)
    # print(linspace(-ma,ma,ms+1))
    angle = estimate_skew_angle(est, np.linspace(-ma, ma, ms+1))
    flat = interpolation.rotate(flat, angle, mode='constant', reshape=0)
    flat = np.amax(flat)-flat
    return flat, angle


def estimate_thresholds(flat, bignore=0.1, escale=1.0, lo=5, hi=90):
    """# estimate low and high thresholds
        bignore: ignore this much of the border for threshold estimation
        escale: for estimating a mask over the text region
        lo: percentile for black estimation
        hi: percentile for white estimation
    """
    d0, d1 = flat.shape
    o0, o1 = int(bignore*d0), int(bignore*d1)
    est = flat[o0:d0-o0, o1:d1-o1]
    if escale > 0:
        # by default, we use only regions that contain significant variance; this makes the
        # percentile-based low and high estimates more reliable
        e = escale
        v = est - filters.gaussian_filter(est, e*20.0)
        v = filters.gaussian_filter(v**2, e*20.0)**0.5
        v = v > 0.3*np.amax(v)
        v = morphology.binary_dilation(v, structure=np.ones((int(e*50), 1)))
        v = morphology.binary_dilation(v, structure=np.ones((1, int(e*50))))
        est = est[v]

    lo1 = stats.scoreatpercentile(est.ravel(), lo)
    hi1 = stats.scoreatpercentile(est.ravel(), hi)
    lo2 = np.percentile(est.ravel(), lo)
    hi2 = np.percentile(est.ravel(), hi)
    print(" lo=%g  hi=%g" % (lo, hi))
    print("lo1=%g hi1=%g" % (lo1, hi1))
    print("lo2=%g hi2=%g" % (lo2, hi2))
    assert lo1 == lo2 and hi1 == hi2
    return lo1, hi1


zoom = 0.5
perc = 80
size = 20
bignore = 0.1
escale = 1.0
defLo = 5
defHi = 90
threshold = 0.5

def binarize(inFile):
    fname = inFile
    raw = ocrolib.read_image_gray(fname)

    # perform image normalization
    image = normalize_raw_image(raw)

    check = check_page(np.amax(image)-image)
    if check is not None:
        print(fname+"SKIPPED"+check+"(use -n to disable this check)")
        return

    # check whether the image is already effectively binarized
    extreme = (np.sum(image < 0.05) + np.sum(image > 0.95)) / np.prod(image.shape)
    if extreme > 0.95:
        comment = "no-normalization"
        flat = image
    else:
        comment = ""
        # if not, we need to flatten it by estimating the local whitelevel
        print("flattening")
        flat = estimate_local_whitelevel(image, zoom, perc, size)

    # estimate skew angle and rotate
    # print("estimating skew angle")
    # flat, angle = estimate_skew(flat, args.bignore, args.maxskew, args.skewsteps)
    angle = 0.0

    # estimate low and high thresholds
    print("estimating thresholds")
    lo, hi = estimate_thresholds(flat, bignore, escale, defLo, defHi)
    print("lo=%5.3f (%g)" % (lo, defLo))
    print("hi=%5.3f (%g)" % (hi, defHi))

    # rescale the image to get the gray scale image
    print("rescaling")
    flat -= lo
    flat /= (hi-lo)
    flat = np.clip(flat, 0, 1)
    bin = 1 * (flat > threshold)

    # output the normalized grayscale and the thresholded images
    print("%s lo-hi (%.2f %.2f) angle %4.1f %s" % (fname, lo, hi, angle, comment))
    print("writing")

    ocrolib.write_image_binary(binFile, bin)
    ocrolib.write_image_gray(grayFile, flat)



parser = argparse.ArgumentParser()
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
grayFile = binBase + ".nrm.png"
print("  inFile=%s" % inFile)
print(" binFile=%s" % binFile)
print("grayFile=%s" % grayFile)
print(" outFile=%s" % outFile)
assert inFile and binFile
assert outFile != inFile
assert outFile != binFile


binarize(inFile)


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
print("~~%s %s" % (inFile, im.size))
print("$$ regions=%s=%s" % (regions, sorted(regions.__dict__)))
print("$$ regions.length=%s" % regions.length())

n = regions.length()
for i in range(1, n):

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
