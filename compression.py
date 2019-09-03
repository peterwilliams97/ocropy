#!/usr/bin/env python
import os
from glob import glob
import argparse
"""
    Find high entropy regions in a PDF
"""

# All files are saved in outPdfRoot.
outPdfRoot = "pdf.output"
suffixMasked = "masked.pdf"
suffixPng = "unmasked.png.pdf"
suffixJpg = "unmasked.jpg.pdf"
suffixBgd = "bgd.pdf"

def main():
    # parser = argparse.ArgumentParser()
    # args = parser.parse_args()
    # pdfFiles = args.files

    mask = os.path.join(outPdfRoot, "*.%s" % suffixMasked)
    pdfFiles = glob(mask)
    pdfFiles = [fn for fn in pdfFiles if testedPdf(fn) ]
    pdfFiles.sort(key=lambda fn: (ratio(fn, suffixPng), fn))
    for i, fn in enumerate(pdfFiles):
        size = suffixMB(fn, None)
        sizePng = suffixMB(fn, suffixPng)
        sizeBgd = suffixMB(fn, suffixBgd)
        print("%3d: %4.2f (%4.2f) %5.2f MB %s" % (i, size/sizePng, sizeBgd/sizePng, size, fn))
    # assert False
    print("=" * 80)


def ratio(filename, suffix):
    other = otherPdf(filename, suffix)
    size = fileSizeMB(filename)
    otherSize = fileSizeMB(other)
    return size / otherSize


def testedPdf(filename):
    pngPdf = otherPdf(filename, suffixPng)
    jpgPdf = otherPdf(filename, suffixJpg)
    bgdPdf = otherPdf(filename, suffixBgd)
    return os.path.exists(pngPdf) and os.path.exists(jpgPdf) and os.path.exists(bgdPdf)


def otherPdf(filename, suffix):
    if not suffix:
        return filename
    base = filename[:-len(suffixMasked)]
    return base + suffix


def suffixMB(filename, suffix):
    return fileSizeMB(otherPdf(filename, suffix))


def fileSizeMB(filename):
    return os.path.getsize(filename) / 1e6

main()
