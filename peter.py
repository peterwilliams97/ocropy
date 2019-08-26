#!/usr/bin/env python

from PIL import Image, ImageDraw
import random as pyrandom
import sys
import os.path
import shutil
import re
import subprocess
import numpy as np
from glob import glob
import argparse
# from matplotlib.pyplot import imread
from scipy.ndimage import filters, interpolation, morphology, measurements
# from scipy.ndimage.filters import gaussian_filter, uniform_filter, maximum_filter

from scipy import stats
from scipy.misc import imsave

import ocrolib
from ocrolib import hocr, common, psegutils, morph, sl
from ocrolib.toplevel import *

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


maxlines = 300
noise = 8 # "noise threshold for removing small components from lines
pad = 3 # padding for extracted line
expand = 3 # expand mask for grayscale extraction
gray = False # output grayscale lines as well which are extracted from the grayscale version of the pages


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("-s", "--start", default=-1, type=int,
                        help="first page in PDF")
    parser.add_argument("-e", "--end", default=-1, type=int,
                        help="last page in PDF")
    parser.add_argument("files", nargs="+",
                        help="input files; glob and @ expansion performed")
    # parser.add_argument("-i", "--input", default="", help="input file")
    # # parser.add_argument("-b", "--bin", default="", help="bin file")

    args = parser.parse_args()
    # inFile = args.input
    os.makedirs(outPdfRoot, exist_ok=True)
    pdfFiles = args.files
    pdfFiles.sort(key=lambda k: (os.path.getsize(k), k))
    for i, inFile in enumerate(pdfFiles):
        print("-" * 80)
        processPdfFile(inFile, args.start, args.end)
        print("Processed %d of %d: %s" % (i + 1, len(pdfFiles), inFile))
    print("Processed %d files %s" % (len(pdfFiles), pdfFiles))


outPdfRoot = "pdf.output"


def processPdfFile(pdfFile, start, end):
    baseName = os.path.basename(pdfFile)
    baseBase, _ = os.path.splitext(baseName)
    outPdfFile = os.path.join(outPdfRoot, baseName)
    outRoot = os.path.join(outPdfRoot, baseBase)
    shutil.copyfile(pdfFile, outPdfFile)
    os.makedirs(outRoot, exist_ok=True)
    retval = runGhostscript(outPdfFile, outRoot)
    assert retval == 0
    fileList = glob(os.path.join(outRoot, "doc-*.png"))
    fileList.sort()

    print("fileList=%d %s" % (len(fileList), fileList))
    for fileNum, origFile in enumerate(fileList):
        page, ok = pageNum(origFile)
        print("#### page=%s ok=%s" % (page, ok))
        if ok:
            if start >= 0 and page < start:
                print("@1", start, end)
                continue
            if end >= 0 and page > end:
                print("@2", start, end)
                continue
        print("@31", start, end)
        processPngFile(outRoot, origFile, fileNum)


gsImageFormat = "doc-%03d.png"
gsImagePattern = r"^doc\-(\d+).png$"
gsImageRegex = re.compile(gsImagePattern)


def pageNum(pngPath):
    name = os.path.basename(pngPath)
    m = gsImageRegex.search(name)
    print("pageNum:", pngPath,name, m)
    if m is None:
        return 0, False
    return int(m.group(1)), True


def runGhostscript(pdf, outputDir):
    """runGhostscript runs Ghostscript on file `pdf` to create file one png file per page in
        directory `outputDir`.
    """
    print("runGhostscript: pdf=%s outputDir=%s" % (pdf, outputDir))
    outputPath = os.path.join(outputDir, gsImageFormat)
    output = "-sOutputFile=%s" % outputPath
    cmd = ["gs",
           "-dSAFER",
           "-dBATCH",
           "-dNOPAUSE",
           "-r300",
           "-sDEVICE=png16m",
           "-dTextAlphaBits=1",
           "-dGraphicsAlphaBits=1",
           output,
           pdf]

    print("runGhostscript: cmd=%s" % cmd)
    print("%s" % ' '.join(cmd))
    os.makedirs(outputDir, exist_ok=True)
    # p = subprocess.Popen(cmd, shell=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    p = subprocess.Popen(cmd, shell=False)

    retval = p.wait()
    print("retval=%d" % retval)
    print("%s" % ' '.join(cmd))
    print(" outputDir=%s" % outputDir)
    print("outputPath=%s" % outputPath)
    assert os.path.exists(outputDir)

    return retval


def processPngFile(outRoot, origFile, fileNum):
    baseName = os.path.basename(origFile)
    baseBase, _ = os.path.splitext(baseName)
    outDir = os.path.join(outRoot, "%s.%03d" % (baseBase, fileNum))
    inFile = os.path.join(outDir, baseName)

    os.makedirs(outDir, exist_ok=True)
    shutil.copy(origFile, inFile)

    inBase, _ = ocrolib.allsplitext(inFile)
    print("**  inBase=%s" % inBase)
    # print("** binBase=%s" % binBase)

    fname = inFile
    outputdir = inBase
    binFile = inBase + ".bin.png"
    outFile = inBase + ".out.png"
    outRoot2, outDir2 = os.path.split(outRoot)
    outFile2 = os.path.join(outRoot2, "%s.out" % outDir2, baseName)
    print("outFile2=%s" % outFile2)
    # assert False
    grayFile = inBase + ".nrm.png"
    psegFile = inBase + ".pseg.png"
    print("  inFile=%s" % inFile)
    print(" binFile=%s" % binFile)
    print("grayFile=%s" % grayFile)
    print(" outFile=%s" % outFile)
    assert inFile and binFile
    assert outFile != inFile
    assert outFile != binFile

    if not binarize(inFile, binFile, grayFile):
        print("Couldn't binarize %s" % inFile)
        return False

    image = ocrolib.read_image_binary(binFile)
    print("$$ %s=%s" % (binFile, desc(image)))
    height, width = image.shape
    binary = image

    # @@1

    checktype(binary, ABINARY2)
    check = check_page(np.amax(binary) - binary)
    if check is not None:
        print("%s SKIPPED %s (use -n to disable this check)" %(fname, check))
        return

    # if args.gray:
    #     if os.path.exists(base+".nrm.png"):
    #         gray = ocrolib.read_image_gray(base+".nrm.png")
    #         checktype(gray, GRAYSCALE)
    #     else:
    #         print_error("Grayscale version %s.nrm.png not found. Use ocropus-nlbin for creating " +
    #                     "normalized grayscale version of the pages as well." % base)
    #         return

    binary = 1 - binary  # invert

    scale = psegutils.estimate_scale(binary)
    print("scale %f" % scale)
    if np.isnan(scale) or scale > 1000.0:
        print("%s: bad scale (%g); skipping\n" % (fname, scale))
        return

    # find columns and text lines
    print("computing segmentation")
    segmentation = compute_segmentation(binary, scale)
    if np.amax(segmentation) > maxlines:
        print("%s: too many lines %g" % (fname, np.amax(segmentation)))
        return

    print("number of lines %g" % np.amax(segmentation))

    # compute the reading order
    print("finding reading order")
    lines = psegutils.compute_lines(segmentation, scale)
    order = psegutils.reading_order([l.bounds for l in lines])
    lsort = psegutils.topsort(order)
    print("$$ lsort = %d = %s" % (len(lsort), lsort[:10]))

    # renumber the labels so that they conform to the specs

    nlabels = np.amax(segmentation) + 1
    renumber = np.zeros(nlabels, 'i')
    for i, v in enumerate(lsort):
        renumber[lines[v].label] = 0x010000+(i+1)
    segmentation = renumber[segmentation]

    # finally, output everything
    print("writing lines")
    if not os.path.exists(outputdir):
        os.mkdir(outputdir)
    lines = [lines[i] for i in lsort]
    ocrolib.write_page_segmentation("%s.pseg.png" % outputdir, segmentation)
    cleaned = ocrolib.remove_noise(binary, noise)
    for i, l in enumerate(lines):
        binline = psegutils.extract_masked(1-cleaned, l, pad=pad, expand=expand)
        ocrolib.write_image_binary("%s/01%04x.bin.png" % (outputdir, i+1), binline)
        # if args.gray:
        #     grayline = psegutils.extract_masked(
        #         gray, l, pad=args.pad, expand=args.expand)
        #     ocrolib.write_image_gray("%s/01%04x.nrm.png" % (outputdir, i+1), grayline)
    print("%6d  %s %4.1f %d" % (i, fname,  scale,  len(lines)))

    # to proceed, we need a pseg file and a subdirectory containing text lines
    assert os.path.exists(psegFile), "%s: no such file" % psegFile
    assert os.path.isdir(inBase), "%s: no such directory" % inBase

    # iterate through the text lines in reading order, based on the page segmentation file
    pseg = ocrolib.read_page_segmentation(psegFile)
    print("$$ %s=%s" % (psegFile, desc(pseg)))

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
        # print("%5d: 0x%05X %s %d x %d" %
        #       (i, id, [y0, x0, y1, x1], y1 - y0, x1 - x0))

        draw = ImageDraw.Draw(im)
        draw.rectangle((x0, y0, x1, y1), outline=(255, 0, 0), width=3)
        draw.rectangle((x0, y0, x1, y1), outline=(0, 0, 255), width=0)
        # draw.rectangle((x0, y0, x1, y1), outline=255, width=5)
        # draw.rectangle((x0, y0, x1, y1), outline=10,  width=1)
        del draw

    # write to stdout
    im.save(outFile, "PNG")
    print("outFile2=%s" % outFile2)
    outDir2 = os.path.dirname(outFile2)
    os.makedirs(outDir2, exist_ok=True)
    im.save(outFile2, "PNG")
    assert os.path.exists(outFile2)


def compute_segmentation(binary, scale):
    """Given a binary image, compute a complete segmentation into lines, computing both columns and
        text lines.
    """
    print("$$ compute_segmentation: %s %g" % (desc(binary), scale))
    binary = np.array(binary, 'B')

    # start by removing horizontal black lines, which only interfere with the rest of the page
    # segmentation
    binary = remove_hlines(binary, scale)

    # do the column finding
    print("computing column separators")
    colseps, binary = compute_colseps(binary, scale)

    # now compute the text line seeds
    print("computing lines")
    bottom, top, boxmap = compute_gradmaps(binary, scale)
    seeds = compute_line_seeds(binary, bottom, top, colseps, scale)
    DSAVE("seeds", [bottom, top, boxmap])

    # spread the text line seeds to all the remaining components
    print("propagating labels")
    llabels = morph.propagate_labels(boxmap, seeds, conflict=0)
    print("spreading labels")
    spread = morph.spread_labels(seeds, maxdist=scale)
    llabels = np.where(llabels > 0, llabels, spread*binary)
    segmentation = llabels * binary
    print("$$ llabels: %s" % desc(llabels))
    print("$$ segmentation: %s" % desc(segmentation))
    return segmentation


################################################################
### Text Line Finding.
###
### This identifies the tops and bottoms of text lines by
### computing gradients and performing some adaptive thresholding.
### Those components are then used as seeds for the text lines.
################################################################

def compute_gradmaps(binary, scale, hscale=1.0, vscale=1.0, usegauss=False):
    """ usegauss: use gaussian instead of uniform"""
    # use gradient filtering to find baselines
    boxmap = psegutils.compute_boxmap(binary, scale)
    cleaned = boxmap * binary
    DSAVE("cleaned", cleaned)
    if usegauss:
        # this uses Gaussians
        grad = filters.gaussian_filter(1.0*cleaned, (vscale*0.3*scale, hscale*6*scale), order=(1, 0))
    else:
        # this uses non-Gaussian oriented filters
        grad = filters.gaussian_filter(1.0*cleaned, (max(4, vscale*0.3*scale),
                                             hscale*scale), order=(1, 0))
        grad = filters.uniform_filter(grad, (vscale, hscale*6*scale))
    bottom = ocrolib.norm_max((grad < 0)*(-grad))
    top = ocrolib.norm_max((grad > 0)*grad)
    return bottom, top, boxmap


def compute_line_seeds(binary, bottom, top, colseps, scale,
                       threshold=0.2, vscale=1.0,):
    """Base on gradient maps, computes candidates for baselines and xheights.  Then, it marks the
       regions between the two as a line seed.
    """
    t = threshold
    vrange = int(vscale*scale)
    bmarked = filters.maximum_filter( bottom == filters.maximum_filter(bottom, (vrange, 0)), (2, 2))
    bmarked = bmarked*(bottom > t*np.amax(bottom)*t)*(1-colseps)
    tmarked = filters.maximum_filter(top == filters.maximum_filter(top, (vrange, 0)), (2, 2))
    tmarked = tmarked*(top > t*np.amax(top)*t/2)*(1-colseps)
    tmarked = filters.maximum_filter(tmarked, (1, 20))
    seeds = np.zeros(binary.shape, 'i')
    delta = max(3, int(scale/2))
    for x in range(bmarked.shape[1]):
        transitions = sorted([(y, 1) for y in find(bmarked[:, x])] +
                             [(y, 0) for y in find(tmarked[:, x])])[::-1]
        transitions += [(0, 0)]
        for l in range(len(transitions)-1):
            y0, s0 = transitions[l]
            if s0 == 0:
                continue
            seeds[y0-delta:y0, x] = 1
            y1, s1 = transitions[l+1]
            if s1 == 0 and (y0-y1) < 5*scale:
                seeds[y1:y0, x] = 1
    seeds = filters.maximum_filter(seeds, (1, int(1+scale)))
    seeds = seeds*(1-colseps)
    DSAVE("lineseeds", [seeds, 0.3*tmarked+0.7*bmarked, binary])
    seeds, _ = morph.label(seeds)
    return seeds


####

def remove_hlines(binary, scale, maxsize=10):
    labels, _ = morph.label(binary)
    objects = morph.find_objects(labels)
    for i, b in enumerate(objects):
        if sl.width(b) > maxsize * scale:
            labels[b][labels[b] == i+1] = 0
    return np.array(labels != 0, 'B')


def find(condition):
    "Return the indices where ravel(condition) is true"
    res, = np.nonzero(np.ravel(condition))
    return res


def desc(a):
    return "%s:%s" % (list(a.shape), a.dtype)

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


def compute_colseps(binary, scale, maxcolseps=3, maxseps=0):
    """Computes column separators either from vertical black lines or whitespace."""
    print("considering at most %g whitespace column separators" % maxcolseps)
    colseps = compute_colseps_conv(binary, scale)
    # DSAVE("colwsseps", 0.7*colseps+0.3*binary)
    if maxseps > 0:
        print("considering at most %g black column separators" % maxseps)
        seps = compute_separators_morph(binary, scale)
        # DSAVE("colseps", 0.7*seps+0.3*binary)
        #colseps = compute_colseps_morph(binary,scale)
        colseps = np.maximum(colseps, seps)
        binary = np.minimum(binary, 1 - seps)
    # binary, colseps = apply_mask(binary, colseps) !@#$
    return colseps, binary


def compute_colseps_conv(binary, scale=1.0,
                         csminheight=10, # minimum column height (units=scale)
                         maxcolseps=3,  # maximum # whitespace column separators
     ):
    """Find column separators by convolution and thresholding."""
    h, w = binary.shape
    # find vertical whitespace by thresholding
    smoothed = filters.gaussian_filter(1.0*binary, (scale, scale*0.5))
    smoothed = filters.uniform_filter(smoothed, (5.0*scale, 1))
    thresh = (smoothed < np.amax(smoothed)*0.1)
    DSAVE("1thresh", thresh)
    # find column edges by filtering
    grad = filters.gaussian_filter(1.0*binary, (scale, scale*0.5), order=(0, 1))
    grad = filters.uniform_filter(grad, (10.0*scale, 1))
    # grad = abs(grad) # use this for finding both edges
    grad = (grad > 0.5*np.amax(grad))
    DSAVE("2grad", grad)
    # combine edges and whitespace
    seps = np.minimum(thresh, filters.maximum_filter(grad, (int(scale), int(5*scale))))
    seps = filters.maximum_filter(seps, (int(2*scale), 1))
    DSAVE("3seps", seps)
    # select only the biggest column separators
    seps = morph.select_regions(seps, sl.dim0, min=csminheight*scale, nbest=maxcolseps)
    DSAVE("4seps", seps)
    return seps


def apply_mask(binary, colseps):
    try:
        mask = ocrolib.read_image_binary(base+".mask.png")
    except IOError:
        raise  # !@#$
        return binary, colseps
    masked_seps = np.maximum(colseps, mask)
    binary = np.minimum(binary, 1-masked_seps)
    # DSAVE("masked_seps", masked_seps)
    return binary, masked_seps


def normalize_raw_image(raw):
    """ perform image normalization """
    image = raw - np.amin(raw)
    if np.amax(image) == np.amin(image):
        # print("# image is empty: %s" % (fname))
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

def binarize(inFile, binFile, grayFile):
    fname = inFile
    raw = ocrolib.read_image_gray(inFile)

    # perform image normalization
    image = normalize_raw_image(raw)
    if image is None:
       print("  # image is empty: %s" % (inFile))
       return False

    check = check_page(np.amax(image)-image)
    if check is not None:
        print(fname+"SKIPPED"+check+"(use -n to disable this check)")
        return False

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
    return True


debug = False

def DSAVE(title, image):
    if not debug:
        return
    if type(image) == list:
        assert len(image) == 3
        image = np.transpose(np.array(image), [1, 2, 0])
    fname = "_%s.png" % title
    print("debug " + fname)
    imsave(fname, image.astype('float'))


main()
