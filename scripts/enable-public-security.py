#!/usr/bin/env python3
"""Enable public-only GitHub settings after an authorized visibility change."""
import json
import subprocess

REPO = 'as19814/terraform-provider-arin'


def api(path, method='GET', payload=None):
    command = ['gh', 'api', f'repos/{REPO}/{path}'.rstrip('/'), '--method', method]
    if payload is not None:
        command += ['--input', '-']
    result = subprocess.run(command, input=json.dumps(payload) if payload is not None else None,
                            text=True, capture_output=True, check=True)
    return json.loads(result.stdout) if result.stdout.strip() else None


if api('')['private']:
    raise SystemExit('Repository is private. This script does not change visibility.')
api('actions/permissions/fork-pr-contributor-approval', 'PUT',
    {'approval_policy': 'all_external_contributors'})
api('private-vulnerability-reporting', 'PUT')
approval = api('actions/permissions/fork-pr-contributor-approval')
reporting = api('private-vulnerability-reporting')
if approval.get('approval_policy') != 'all_external_contributors' or not reporting.get('enabled'):
    raise SystemExit('Public security settings did not match the requested values.')
print('External contributor approval and private vulnerability reporting are enabled.')
