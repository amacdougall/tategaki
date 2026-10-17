from tategaki.paginate import Cut, Illustration, merge_bands, pack, paginate


def columns(n, right=1000.0, width=38.0, pitch=66.0):
    return [(right - i * pitch - width, right - i * pitch) for i in range(n)]


def test_merge_orders_right_to_left_and_unions_overlaps():
    assert merge_bands([(0, 10), (5, 20), (30, 40)]) == [(30, 40), (0, 20)]


def test_pack_fills_width_and_cuts_in_gaps():
    bands = merge_bands(columns(40))
    pages = pack(bands, width=1092)
    per_page = [sum(1 for b in bands if p.left <= b[0] and b[1] <= p.right) for p in pages]
    assert sum(per_page) == 40
    assert per_page[0] == 16  # (1092 - 38) // 66 + 1
    for p in pages:
        assert p.right - p.left <= 1092
        # clip bounds stay inside the neighbouring gaps
        assert p.clip_left < p.left and p.clip_right > p.right
        assert not any(p.clip_left < b[1] and b[0] < p.left for b in bands if b[1] <= p.left)


def test_oversized_band_gets_own_page():
    pages = pack([(0, 2000)], width=1092)
    assert len(pages) == 1 and isinstance(pages[0], Cut)


def test_marker_splits_strip():
    bands = columns(6)
    marker_x = bands[2][0] - 10  # between column 3 and 4
    pages = paginate(bands, [(0, marker_x)], width=1092)
    assert [type(p) for p in pages] == [Cut, Illustration, Cut]
    assert pages[0].left >= marker_x and pages[2].right <= marker_x
