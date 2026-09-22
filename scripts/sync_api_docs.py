#!/usr/bin/env python3
"""Snapshot the curated public ARIN documentation listed in sources.json."""
import base64
import hashlib
import os
import zipfile
import json
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urljoin

import requests
from bs4 import BeautifulSoup
from markdownify import markdownify

ROOT = Path(__file__).resolve().parents[1] / 'docs' / 'reference' / 'arin-api'


def main():
    sources = json.loads((ROOT / 'sources.json').read_text())
    records = []
    links = []
    session = requests.Session()
    session.headers['User-Agent'] = 'ARIN-provider-documentation-snapshot/1.0'
    for source in sources:
        url, name = source['url'], source['file']
        response = session.get(url, timeout=60)
        response.raise_for_status()
        retrieved = datetime.now(timezone.utc).isoformat()
        target = ROOT / name
        target.parent.mkdir(parents=True, exist_ok=True)
        if name.endswith('.md'):
            soup = BeautifulSoup(response.content, 'html.parser')
            main_text = soup.select_one('#main-text')
            if main_text is None:
                raise ValueError(f'Missing article body: {url}')
            title = soup.find('h1').get_text(' ', strip=True)
            for a in main_text.select('a[href]'):
                absolute = urljoin(response.url, a['href'])
                links.append({'source': url, 'label': a.get_text(' ', strip=True), 'url': absolute})
                a['href'] = absolute
            for index, img in enumerate(main_text.select('img[src]')):
                src = urljoin(response.url, img['src'])
                if src.startswith('data:image/'):
                    metadata, encoded = src.split(',', 1)
                    extension = metadata.split('/')[1].split(';')[0]
                    if extension not in ('png', 'jpeg', 'gif', 'webp') or ';base64' not in metadata:
                        raise ValueError(f'Unexpected inline image: {metadata}')
                    asset = ROOT / 'media' / f'{target.stem}-{index}.{extension}'
                    asset.parent.mkdir(exist_ok=True)
                    asset.write_bytes(base64.b64decode(encoded))
                    img['src'] = os.path.relpath(asset, target.parent)
                else:
                    img['src'] = src
            for unwanted in main_text.select('script, style'):
                unwanted.decompose()
            body = markdownify(str(main_text), heading_style='ATX', code_language='', strip=['script', 'style'])
            text = f'# {title}\n\nSource: {response.url}\n\nRetrieved: {retrieved}\n\nPublisher: American Registry for Internet Numbers (ARIN).\n\nLocal reading copy; formatting and punctuation normalized. Upstream is authoritative.\n\n---\n\n{body.strip()}\n'
            target.write_text(text.replace(chr(0x2014), ' - '))
        else:
            if 'text/html' in response.headers.get('Content-Type', ''):
                raise ValueError(f'Expected attachment, received HTML: {url}')
            title = source.get('title', name)
            target.write_bytes(response.content)
            if name.endswith('.zip'):
                extracted = ROOT / 'schemas' / 'extracted'
                with zipfile.ZipFile(target) as archive:
                    if archive.testzip() is not None:
                        raise ValueError(f'Corrupt archive: {url}')
                    for member in archive.infolist():
                        if member.is_dir():
                            continue
                        destination = (extracted / member.filename).resolve()
                        if not destination.is_relative_to(extracted.resolve()):
                            raise ValueError(f'Unsafe archive path: {member.filename}')
                        destination.parent.mkdir(parents=True, exist_ok=True)
                        destination.write_bytes(archive.read(member))
        record = dict(source, title=title, resolved_url=response.url, retrieved_at=retrieved,
                      content_type=response.headers.get('Content-Type'),
                      upstream_sha256=hashlib.sha256(response.content).hexdigest(),
                      local_sha256=hashlib.sha256(target.read_bytes()).hexdigest(),
                      bytes=target.stat().st_size)
        records.append(record)
        print(f'{name}: {record["bytes"]} bytes', flush=True)
    (ROOT / 'manifest.json').write_text(json.dumps(records, indent=2, ensure_ascii=True) + '\n')
    (ROOT / 'links.json').write_text(json.dumps(links, indent=2, ensure_ascii=True) + '\n')


if __name__ == '__main__':
    main()
