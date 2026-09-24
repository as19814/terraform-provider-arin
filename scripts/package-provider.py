#!/usr/bin/env python3
"""Build release candidates without publishing or signing them."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('version', help='Semantic version, for example 0.1.0-rc.1')
args = parser.parse_args()
if not re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?', args.version):
    raise SystemExit('Invalid version')
out = ROOT / 'dist' / args.version
out.mkdir(parents=True, exist_ok=False)
try:
    with tempfile.TemporaryDirectory(prefix='arin-release-') as directory:
        for system, arch in [('linux', 'amd64'), ('linux', 'arm64'), ('darwin', 'amd64'), ('darwin', 'arm64'), ('windows', 'amd64')]:
            binary_name = 'terraform-provider-arin_v'+args.version+('_x6.exe' if system == 'windows' else '_x6')
            binary = Path(directory) / binary_name
            env = dict(os.environ, CGO_ENABLED='0', GOOS=system, GOARCH=arch)
            subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false', '-ldflags=-s -w -X main.version='+args.version, '-o', str(binary), '.'], cwd=ROOT, env=env, check=True)
            archive = out / f'terraform-provider-arin_{args.version}_{system}_{arch}.zip'
            with zipfile.ZipFile(archive, 'w', compression=zipfile.ZIP_DEFLATED) as zipped:
                info = zipfile.ZipInfo(binary_name, date_time=(1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.external_attr = 0o100755 << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                zipped.writestr(info, binary.read_bytes())
                for name in ('LICENSE', 'NOTICE'):
                    info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
                    info.create_system = 3
                    info.external_attr = 0o100644 << 16
                    info.compress_type = zipfile.ZIP_DEFLATED
                    zipped.writestr(info, (ROOT / name).read_bytes())
            print(archive.name, flush=True)
    manifest = out / f'terraform-provider-arin_{args.version}_manifest.json'
    manifest.write_text(json.dumps({'version': 1, 'metadata': {'protocol_versions': ['6.0']}}, indent=2)+'\n')
    sums = ''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in sorted(out.iterdir()))
    (out / f'terraform-provider-arin_{args.version}_SHA256SUMS').write_text(sums)
except BaseException:
    # Partial artifacts are retained for diagnosis, never described as a release.
    (out / 'INCOMPLETE').write_text('Packaging failed. Do not distribute these artifacts.\n')
    raise
print(out)
