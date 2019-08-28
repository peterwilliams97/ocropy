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
from skimage.filters.rank import entropy
from skimage.morphology import disk
from skimage.io import imread, imsave
from skimage.util import img_as_ubyte

from ocrolib.toplevel import desc

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
    parser.add_argument("-n", "--needed", default=1, type=int,
                        help="min number of pages required")
    parser.add_argument("files", nargs="+",
                        help="input files; glob and @ expansion performed")
    parser.add_argument("-f", "--force", action="store_true",
                        help="force processing of PDF file")

    args = parser.parse_args()
    os.makedirs(outPdfRoot, exist_ok=True)
    pdfFiles = args.files
    pdfFiles.sort(key=lambda k: (os.path.getsize(k), k))
    processedFiles = []
    for i, inFile in enumerate(pdfFiles):
        print("-" * 80)
        if not processPdfFile(inFile, args.start, args.end, args.needed, args.force):
            continue
        processedFiles.append(inFile)
        print("Processed %d (%d of %d): %s" % (len(processedFiles), i + 1, len(pdfFiles), inFile))
    print("=" * 80)
    print("Processed %d files %s" % (len(processedFiles), processedFiles))


outPdfRoot = "pdf.output"


def processPdfFile(pdfFile, start, end, needed, force):
    assert needed >= 0, needed
    baseName = os.path.basename(pdfFile)
    baseBase, _ = os.path.splitext(baseName)
    outPdfFile = os.path.join(outPdfRoot, baseName)
    outRoot = os.path.join(outPdfRoot, baseBase)

    if not force and os.path.exists(outPdfFile):
        print("%s exists. skipping" % outPdfFile)
        return False

    os.makedirs(outRoot, exist_ok=True)
    retval = runGhostscript(pdfFile, outRoot)
    assert retval == 0
    fileList = glob(os.path.join(outRoot, "doc-*.png"))
    fileList.sort()

    print("fileList=%d %s" % (len(fileList), fileList))
    numPages = 0
    for fileNum, origFile in enumerate(fileList):
        page, ok = pageNum(origFile)
        print("#### page=%s ok=%s" % (page, ok))
        if ok:
            if start >= 0 and page < start:
                print("@1", [start, end])
                continue
            if end >= 0 and page > end:
                if not (needed >= 0 and numPages < needed):
                    print("@2", [start, end], [numPages, needed])
                    continue
        print("@31", start, end)
        ok = processPngFile(outRoot, origFile, fileNum)
        if ok:
            numPages += 1
    assert numPages > 0

    if numPages == 0:
        print("~~ No pages processed")
        return False

    shutil.copyfile(pdfFile, outPdfFile)
    return True


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

    outRoot2, outDir2 = os.path.split(outRoot)
    outFile2 = os.path.join(outRoot2, "%s.entropy" % outDir2, baseName)
    print("outFile2=%s" % outFile2)
    # assert False

    image = imread(origFile, as_gray=True)
    image = img_as_ubyte(image)
    print("  image=%s" % desc(image))
    print("+" * 80)
    entImage = entropy(image, disk(125))
    print("entImage=%s" % desc(entImage))
    entImage = normalize(entImage)
    print("entImage=%s" % desc(entImage))
    entImage = img_as_ubyte(entImage)
    print("entImage=%s" % desc(entImage))

    outDir2 = os.path.dirname(outFile2)
    os.makedirs(outDir2, exist_ok=True)
    imsave(outFile2, entImage)

    return True


def normalize(a):
    return a / 10
    mn = np.amin(a)
    mx = np.amax(a)
    print("normalize: min=%g max=%g" % (mn, mx))
    a = (a - mn) / (mx - mn)
    return a

main()
