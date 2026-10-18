"""tategaki: build and deploy vertical-text bundles for the Kindle reader."""

import argparse
import sys
import webbrowser
from pathlib import Path

from .config import ConfigError, Library


def _floats(s: str) -> list[float]:
    return [float(x) for x in s.split(',') if x]


def _paths(s: str) -> list[Path]:
    return [Path(x).expanduser() for x in s.split(',') if x]


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(prog='tategaki', description=__doc__)
    sub = ap.add_subparsers(dest='cmd', required=True)

    b = sub.add_parser('build', help='convert books whose inputs changed')
    b.add_argument('books', nargs='*', help='book ids (folder names); default: all')
    b.add_argument('--force', action='store_true', help='rebuild even if up to date')

    d = sub.add_parser('deploy', help='send built books and settings to the Kindle')
    d.add_argument('books', nargs='*')
    d.add_argument('--settings-only', action='store_true', help='only config.json, per-book settings and the Anki word list')
    d.add_argument('--usb', type=Path, metavar='VOLUME', help='copy to a mounted Kindle instead of SSH')
    d.add_argument('--force', action='store_true', help='resend books even if unchanged')

    c = sub.add_parser('convert', help='low-level: one EPUB to one .jbk with explicit settings')
    c.add_argument('epub', type=Path)
    c.add_argument('-o', '--output', type=Path, required=True)
    c.add_argument('--font', type=Path, required=True)
    c.add_argument('--font-size-pt', type=float, default=9)
    c.add_argument('--line-height', type=float, default=1.75)
    c.add_argument('--furigana', choices=['all', 'rare', 'none'], default='rare')
    c.add_argument('--rare-threshold', type=int, default=10000)
    c.add_argument('--spreads', choices=['split', 'rotate', 'fit'], default='split')
    c.add_argument('--gamma', type=float, default=1.0)
    c.add_argument('--no-quantize', action='store_true')
    c.add_argument('--pages', type=int, help='stop after this many pages')
    c.add_argument('--profile', default='pw11')

    p = sub.add_parser('proof', help='typography proof sheets in one .jbk')
    p.add_argument('epub', type=Path)
    p.add_argument('--pages', type=int, default=4)
    p.add_argument('--font-size-pt', type=_floats, default=[9.0])
    p.add_argument('--font', type=_paths, help='comma-separated font files (default: library font)')
    p.add_argument('--line-height', type=_floats, default=[1.75])
    p.add_argument('-o', '--output', type=Path, help='default: $TATEGAKI_HOME/proof/proof.jbk')
    p.add_argument('--deploy', action='store_true', help='send to the Kindle as book "proof"')

    v = sub.add_parser('preview', help='open an HTML preview of a .jbk (or book id)')
    v.add_argument('target', help='a .jbk path or a book id')
    v.add_argument('-o', '--output', type=Path, help='output folder (default: next to the .jbk)')
    v.add_argument('--no-open', action='store_true')

    args = ap.parse_args(argv)
    try:
        return _run(args)
    except ConfigError as e:
        print(f'config error: {e}', file=sys.stderr)
        return 2


def _run(args) -> int:
    from . import library
    from .render import Profile, RenderSettings

    if args.cmd == 'build':
        library.build(Library.load(), args.books, force=args.force)
    elif args.cmd == 'deploy':
        library.deploy(Library.load(), args.books, settings_only=args.settings_only, usb=args.usb, force=args.force)
    elif args.cmd == 'convert':
        from .convert import convert
        s = RenderSettings(font=args.font, font_size_pt=args.font_size_pt, line_height=args.line_height,
                           furigana=args.furigana, rare_threshold=args.rare_threshold, spreads=args.spreads,
                           gamma=args.gamma, quantize=not args.no_quantize)
        m = convert(args.epub, args.output, s, Profile.load(args.profile), page_limit=args.pages)
        print(f'{m["page_count"]} pages -> {args.output}')
    elif args.cmd == 'proof':
        from .proof import proof
        lib = Library.load()
        fonts = args.font or [Path(lib.render['font']) if Path(lib.render['font']).is_absolute()
                              else lib.root / lib.render['font']]
        out = args.output or lib.root / 'proof' / 'proof.jbk'
        out.parent.mkdir(parents=True, exist_ok=True)
        m = proof(args.epub, out, Profile.load(lib.device['profile']), args.pages, fonts,
                  args.font_size_pt, args.line_height, furigana=lib.render['furigana'])
        print(f'{m["page_count"]} pages -> {out}')
        if args.deploy:
            target = library.SshTarget(lib.device['ssh_host'])
            target.mkdir(f'{library.DEVICE_ROOT}/books')
            target.put(out, f'{library.DEVICE_ROOT}/books/proof.jbk')
            print('deployed as book "proof"')
    elif args.cmd == 'preview':
        from .preview import build_preview
        jbk = Path(args.target)
        if not jbk.suffix == '.jbk':
            jbk = Library.load().book(args.target).jbk
        out = args.output or jbk.with_name(jbk.stem + '-preview')
        index = build_preview(jbk, out)
        print(index)
        if not args.no_open:
            webbrowser.open(index.as_uri())
    return 0


if __name__ == '__main__':
    sys.exit(main())
