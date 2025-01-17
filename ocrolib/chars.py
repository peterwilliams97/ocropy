# -*- encoding: utf-8 -*-

import re

# common character sets

digits = "0123456789"
letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
symbols = r"""!"#$%&'()*+,-./:;<=>?@[\]^_`{|}~"""
ascii = digits+letters+symbols

xsymbols = """€¢£»«›‹÷©®†‡°∙•◦‣¶§÷¡¿▪▫"""
german = "ÄäÖöÜüß"
french = "ÀàÂâÆæÇçÉéÈèÊêËëÎîÏïÔôŒœÙùÛûÜüŸÿ"
turkish = "ĞğŞşıſ"
greek = "ΑαΒβΓγΔδΕεΖζΗηΘθΙιΚκΛλΜμΝνΞξΟοΠπΡρΣσςΤτΥυΦφΧχΨψΩω"
portuguese = "ÁÃÌÍÒÓÕÚáãìíòóõú"
telugu = " ఁంఃఅఆఇఈఉఊఋఌఎఏఐఒఓఔకఖగఘఙచఛజఝఞటఠడఢణతథదధనపఫబభమయరఱలళవశషసహఽాిీుూృౄెేైొోౌ్ౘౙౠౡౢౣ౦౧౨౩౪౫౬౭౮౯"

default = ascii+xsymbols+german+french+portuguese

european = default+turkish+greek

# List of regular expressions for normalizing Unicode text.
# Cleans up common homographs. This is mostly used for
# training text.

# Note that the replacement of pretty much all quotes with
# ASCII straight quotes and commas requires some
# postprocessing to figure out which of those symbols
# represent typographic quotes. See `requote`

# TODO: We may want to try to preserve more shape; unfortunately,
# there are lots of inconsistencies between fonts. Generally,
# there seems to be left vs right leaning, and top-heavy vs bottom-heavy

replacements = [
    (u'[_~#]', u"~"), # OCR control characters
    (u'"', u"''"), # typewriter double quote
    (u"`", u"'"), # grave accent
    (u'[“”]', u"''"), # fancy quotes
    (u"´",u"'"), # acute accent
    (u"[‘’]",u"'"), # left single quotation mark
    (u"[“”]",u"''"), # right double quotation mark
    (u"“",u"''"), # German quotes
    (u"„",u",,"), # German quotes
    (u"…",u"..."), # ellipsis
    (u"′",u"'"), # prime
    (u"″",u"''"), # double prime
    (u"‴",u"'''"), # triple prime
    (u"〃",u"''"), # ditto mark
    (u"µ",u"μ"), # replace micro unit with greek character
    (u"[–—]",u"-"), # variant length hyphens
    (u"ﬂ",u"fl"), # expand Unicode ligatures
    (u"ﬁ",u"fi"),
    (u"ﬀ",u"ff"),
    (u"ﬃ",u"ffi"),
    (u"ﬄ",u"ffl"),
]

def requote(s):
    return re.sub(r"''", '"', s)

def requote_fancy(s, germanic=0):
    if germanic:
        # germanic quoting style reverses the shapes
        # straight double quotes
        s = re.sub(r"\s+''", u"”",s)
        s = re.sub(u"''\s+", u"“",s)
        s = re.sub(r"\s+,,", u"„",s)
        # straight single quotes
        s = re.sub(r"\s+'",u"’",s)
        s = re.sub(r"'\s+",u"‘",s)
        s = re.sub(r"\s+,",u"‚",s)
    else:
        # straight double quotes
        s = re.sub(r"\s+''",u"“",s)
        s = re.sub(r"''\s+",u"”",s)
        s = re.sub(r"\s+,,",u"„",s)
        # straight single quotes
        s = re.sub(r"\s+'",u"‘",s)
        s = re.sub(r"'\s+",u"’",s)
        s = re.sub(r"\s+,",u"‚",s)
    return s
