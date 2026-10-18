from types import SimpleNamespace

from PIL import Image

from tategaki.render import sleep_image


def test_sleep_image_fills_the_screen(tmp_path):
    Image.new('RGB', (300, 600), (200, 30, 30)).save(tmp_path / 'cover.jpg')
    book = SimpleNamespace(cover='cover.jpg', path=lambda name: tmp_path / name)
    profile = SimpleNamespace(width=1236, height=1648)
    img = sleep_image(book, profile, 1.0)
    assert img.size == (1236, 1648) and img.mode == 'L'
    assert img.getpixel((0, 824)) == 255          # white letterbox at the sides
    assert img.getpixel((618, 824)) < 255         # the cover in the middle
    assert len(img.getcolors()) <= 16              # 16 gray levels, like illustrations


def test_sleep_image_without_cover():
    book = SimpleNamespace(cover=None, path=None)
    assert sleep_image(book, SimpleNamespace(width=1236, height=1648), 1.0) is None
