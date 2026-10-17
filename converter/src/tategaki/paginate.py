"""Strip slicing (plan §5.1).

A chapter is laid out as one vertical-rl strip that grows leftward. Every
drawn thing (glyph, ruby, image) occupies a horizontal band [x0, x1]. Pages are
cut only in the empty gaps between bands, packing bands greedily from right to
left into the content width. Illustration markers force a cut.
"""

from dataclasses import dataclass

EPS = 0.5
# how far a page image may extend past its outermost em box into the gap
GAP_PAD = 12.0


@dataclass
class Cut:
    """One text page: content spans [left, right] in strip coordinates; the
    page image keeps [clip_left, clip_right], which extend halfway into the
    neighbouring gaps so ink slightly outside the em boxes survives."""
    left: float
    right: float
    clip_left: float = 0.0
    clip_right: float = 0.0


@dataclass
class Illustration:
    index: int  # into the chapter's illustration sources


def merge_bands(bands: list[tuple[float, float]]) -> list[tuple[float, float]]:
    """Union overlapping or touching intervals; returned right to left."""
    spans = sorted((min(a, b), max(a, b)) for a, b in bands if b - a > 0)
    merged: list[list[float]] = []
    for a, b in spans:
        if merged and a <= merged[-1][1] + EPS:
            merged[-1][1] = max(merged[-1][1], b)
        else:
            merged.append([a, b])
    return [(a, b) for a, b in reversed(merged)]


def pack(bands: list[tuple[float, float]], width: float) -> list[Cut]:
    """Greedy right-to-left packing of merged bands (right to left) into pages
    at most `width` wide. A band wider than `width` gets a page of its own."""
    pages: list[Cut] = []
    i = 0
    while i < len(bands):
        right = bands[i][1]
        left = bands[i][0]
        j = i + 1
        while j < len(bands) and bands[j][0] >= right - width - EPS:
            left = bands[j][0]
            j += 1
        gap_left = bands[j][1] if j < len(bands) else left - GAP_PAD * 2
        gap_right = bands[i - 1][0] if i > 0 else right + GAP_PAD * 2
        pages.append(Cut(left=left, right=right,
                         clip_left=max((left + gap_left) / 2, left - GAP_PAD),
                         clip_right=min((right + gap_right) / 2, right + GAP_PAD)))
        i = j
    return pages


def paginate(bands: list[tuple[float, float]], markers: list[tuple[int, float]],
             width: float) -> list[Cut | Illustration]:
    """Pages for a chapter. markers are (illustration index, x); each one splits
    the strip, so text right of it comes first, then the illustration."""
    merged = merge_bands(bands)
    pages: list[Cut | Illustration] = []
    remaining = merged
    for index, x in sorted(markers, key=lambda m: -m[1]):
        before = [b for b in remaining if b[0] >= x - EPS]
        remaining = [b for b in remaining if b[0] < x - EPS]
        pages.extend(pack(before, width))
        pages.append(Illustration(index))
    pages.extend(pack(remaining, width))
    return pages
