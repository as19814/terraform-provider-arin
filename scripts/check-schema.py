#!/usr/bin/env python3
"""Reviewable Terraform schema baseline, excluding documentation-only changes."""
import argparse
import difflib
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
BASELINE = ROOT / 'testdata/provider-schema.json'
parser = argparse.ArgumentParser()
parser.add_argument('--update', action='store_true', help='Explicitly accept the current schema after review')
parser.add_argument('--provider-dir', type=Path, default=ROOT / 'bin', help='Directory containing the provider binary')
args = parser.parse_args()

def structural(value):
    if isinstance(value, dict):
        return {k: structural(v) for k, v in value.items() if k not in ('description', 'description_kind') or isinstance(v, dict)}
    if isinstance(value, list):
        return [structural(v) for v in value]
    return value

with tempfile.TemporaryDirectory(prefix='arin-schema-') as directory:
    work = Path(directory)
    (work / 'main.tf').write_text('terraform {\n  required_providers {\n    arin = { source = "as19814/arin" }\n  }\n}\n')
    config = work / 'dev.tfrc'
    config.write_text('provider_installation {\n dev_overrides {\n "as19814/arin" = '+json.dumps(str(args.provider_dir.resolve()))+'\n }\n direct {}\n}\n')
    env = dict(os.environ, TF_CLI_CONFIG_FILE=str(config), TF_IN_AUTOMATION='1')
    result = subprocess.run(['terraform', 'providers', 'schema', '-json'], cwd=work, env=env, check=True, capture_output=True, text=True)
    schemas = json.loads(result.stdout)['provider_schemas']['registry.terraform.io/as19814/arin']
    rendered = json.dumps(structural(schemas), sort_keys=True, indent=2)+'\n'
if args.update:
    BASELINE.write_text(rendered)
    print('Updated schema baseline; review the diff before committing.')
else:
    previous = BASELINE.read_text()
    if previous != rendered:
        print(''.join(difflib.unified_diff(previous.splitlines(True), rendered.splitlines(True), fromfile='committed schema', tofile='current schema')))
        raise SystemExit('Schema changed: review state/configuration compatibility, then explicitly update the baseline.')
    print('Provider schema matches the reviewed baseline.')
