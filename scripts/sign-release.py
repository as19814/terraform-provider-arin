#!/usr/bin/env python3
"""Sign an already-built release in an isolated, temporary GPG keyring."""
import argparse
import hashlib
import os
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('directory', type=Path)
args = parser.parse_args()
root = args.directory.resolve()
sums = list(root.glob('terraform-provider-arin_*_SHA256SUMS'))
if len(sums) != 1 or (root / 'INCOMPLETE').exists():
    raise SystemExit('Expected one complete release checksum manifest.')
sums = sums[0]
files = set()
for line in sums.read_text().splitlines():
    digest, name = line.split('  ', 1)
    if Path(name).name != name or name in files or not re.fullmatch('[0-9a-f]{64}', digest):
        raise SystemExit('Invalid checksum manifest.')
    path = root / name
    if path.is_symlink() or hashlib.sha256(path.read_bytes()).hexdigest() != digest:
        raise SystemExit('Release checksum mismatch: ' + name)
    files.add(name)
if not files or not any(name.endswith('_manifest.json') for name in files):
    raise SystemExit('Missing release manifest.')
if {p.name for p in root.iterdir()} != files | {sums.name}:
    raise SystemExit('Unexpected files in unsigned release directory.')
fingerprint = os.environ['GPG_FINGERPRINT'].upper()
if not re.fullmatch('[0-9A-F]{40}', fingerprint):
    raise SystemExit('Expected full RSA signing-key fingerprint.')
secret = os.environ['GPG_PRIVATE_KEY']
passphrase = os.environ['PASSPHRASE']
# Child processes do not inherit the signing secrets through their environment.
env = {k: v for k, v in os.environ.items() if k not in ('GPG_PRIVATE_KEY', 'PASSPHRASE')}
with tempfile.TemporaryDirectory(prefix='arin-gpg-') as directory:
    env['GNUPGHOME'] = directory
    gpg = ['gpg', '--batch', '--no-tty']
    try:
        subprocess.run(gpg + ['--import'], input=secret.encode(), env=env, check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        keys = subprocess.check_output(gpg + ['--with-colons', '--list-secret-keys'], env=env, text=True)
        if fingerprint not in [line.split(':')[9] for line in keys.splitlines() if line.startswith('fpr:')]:
            raise SystemExit('Imported key does not match configured fingerprint.')
        subprocess.run(gpg + ['--pinentry-mode', 'loopback', '--passphrase-fd', '0',
                       '--local-user', fingerprint, '--digest-algo', 'SHA256',
                       '--output', str(sums) + '.sig', '--detach-sign', str(sums)],
                       input=(passphrase + '\n').encode(), env=env, check=True)
        subprocess.run(gpg + ['--verify', str(sums) + '.sig', str(sums)], env=env, check=True)
    finally:
        subprocess.run(['gpgconf', '--kill', 'gpg-agent'], env=env, check=False)
print('Release checksums and detached signature verified.')
