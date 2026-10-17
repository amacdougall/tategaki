"""E-ink image processing (plan §5.5, §5.6)."""

from PIL import Image, ImageOps

LEVELS = 16  # the panel is 4-bit


def _palette_image() -> Image.Image:
    pal = Image.new('P', (1, 1))
    grays = [round(i * 255 / (LEVELS - 1)) for i in range(LEVELS)]
    flat = []
    for g in grays:
        flat += [g, g, g]
    flat += [0, 0, 0] * (256 - LEVELS)
    pal.putpalette(flat)
    return pal


_PALETTE = _palette_image()


def gamma(img: Image.Image, g: float) -> Image.Image:
    if g == 1.0:
        return img
    lut = [round(255 * ((i / 255) ** g)) for i in range(256)]
    return img.point(lut)


def quantize(img: Image.Image, dither: bool) -> Image.Image:
    """Reduce an 'L' image to 16 gray levels."""
    if not dither:
        step = 255 / (LEVELS - 1)
        return img.point([round(round(i / step) * step) for i in range(256)])
    q = img.convert('RGB').quantize(palette=_PALETTE, dither=Image.Dither.FLOYDSTEINBERG)
    return q.convert('L')


def process_illustration(img: Image.Image, g: float, quantize_levels: bool = True) -> Image.Image:
    img = ImageOps.exif_transpose(img).convert('L')
    img = gamma(img, g)
    return quantize(img, dither=True) if quantize_levels else img


def fit(img: Image.Image, width: int, height: int) -> Image.Image:
    """Scale to fit width x height (Lanczos), centred on white."""
    scale = min(width / img.width, height / img.height)
    w, h = max(1, round(img.width * scale)), max(1, round(img.height * scale))
    scaled = img.resize((w, h), Image.Resampling.LANCZOS)
    page = Image.new('L', (width, height), 255)
    page.paste(scaled, ((width - w) // 2, (height - h) // 2))
    return page


def illustration_pages(img: Image.Image, width: int, height: int, spreads: str,
                       spread_ratio: float, g: float) -> list[Image.Image]:
    """Full-screen page(s) for one illustration. Landscape images are spreads:
    split (right half first, for right-to-left reading), rotate, or fit."""
    img = ImageOps.exif_transpose(img).convert('L')
    if img.width > img.height * spread_ratio:
        if spreads == 'split':
            mid = img.width // 2
            halves = [img.crop((mid, 0, img.width, img.height)), img.crop((0, 0, mid, img.height))]
            return [process_illustration(fit(h, width, height), g) for h in halves]
        if spreads == 'rotate':
            img = img.rotate(90, expand=True)
    return [process_illustration(fit(img, width, height), g)]
