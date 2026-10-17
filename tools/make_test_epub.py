#!/usr/bin/env python3
"""Write a small vertical-text EPUB for lookup testing (plan Phase 4).

It holds the plan's and fixtures' example sentences (蜘蛛の巣, 擦られた,
悠々, 並々ならぬ, speaker tags, ...), so taps in the simulator or on the device
have known answers.

Usage: python3 tools/make_test_epub.py [OUT_DIR]   (default ~/Tategaki/books/lookup-test)
"""

import os
import sys
import zipfile
from pathlib import Path

PARAGRAPHS = [
    '蜘蛛の巣が張っている。',
    '　壁を擦られた跡がある。誰かが擦ったのだろう。',
    '　彼は悠々と歩いていった。',
    '　並々ならぬ決意で臨んだ。',
    '　食べていますが、まだ足りない。',
    '　<ruby>苛<rt>いら</rt>立<rt>だ</rt></ruby>ちに任せ、<ruby>曇<rt>くも</rt></ruby>る<ruby>鏡<rt>かがみ</rt></ruby>を乱暴に手のひらで拭った。',
    '筆者　えーと……実は今、引っ越しを考えていまして。',
    '栗原　やっぱり私は、殺された四人目がいると思うんですよ。これを見てください。',
    '田中さんは「ちょっと待って！」と言った。',
    '「でもはいらないよ。とにかく明日は宿を探そう。だから、そんなに気を張らないでいいよ」',
    '　そんなことするわけない。床に跪き、こぼれたゴミを拾い集め、突き込んだ。',
    '　<ruby>高<rt>たか</rt></ruby><ruby>須<rt>す</rt></ruby><ruby>竜<rt>りゅう</rt></ruby><ruby>児<rt>じ</rt></ruby>の前髪は、いまやまさしく『ふんわりバングス』。',
]

XHTML = '''<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="ja" class="vrtl">
<head><meta charset="UTF-8"/><title>{title}</title>
<style>html {{ writing-mode: vertical-rl; }}</style></head>
<body>
<h1 id="h">{title}</h1>
{body}
</body></html>
'''

NAV = '''<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="ja">
<head><meta charset="UTF-8"/><title>目次</title></head>
<body><nav epub:type="toc"><ol>
<li><a href="text.xhtml#h">辞書テスト</a></li>
</ol></nav></body></html>
'''

OPF = '''<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id" xml:lang="ja">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:identifier id="id">urn:uuid:tategaki-lookup-test</dc:identifier>
<dc:title>辞書テスト</dc:title>
<dc:creator>Tategaki</dc:creator>
<dc:language>ja</dc:language>
<meta property="dcterms:modified">2026-10-17T00:00:00Z</meta>
</metadata>
<manifest>
<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
<item id="text" href="text.xhtml" media-type="application/xhtml+xml"/>
</manifest>
<spine page-progression-direction="rtl"><itemref idref="text"/></spine>
</package>
'''

CONTAINER = '''<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
<rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>
'''


def main() -> int:
    out_dir = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(os.environ.get('TATEGAKI_HOME', Path.home() / 'Tategaki')) / 'books' / 'lookup-test'
    out_dir.mkdir(parents=True, exist_ok=True)
    out = out_dir / 'lookup-test.epub'
    body = '\n'.join(f'<p>{p}</p>' for p in PARAGRAPHS)
    with zipfile.ZipFile(out, 'w') as z:
        z.writestr(zipfile.ZipInfo('mimetype'), 'application/epub+zip', compress_type=zipfile.ZIP_STORED)
        z.writestr('META-INF/container.xml', CONTAINER)
        z.writestr('OEBPS/content.opf', OPF)
        z.writestr('OEBPS/nav.xhtml', NAV)
        z.writestr('OEBPS/text.xhtml', XHTML.format(title='辞書テスト', body=body))
    print(out)
    return 0


if __name__ == '__main__':
    sys.exit(main())
