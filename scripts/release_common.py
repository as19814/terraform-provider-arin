#!/usr/bin/env python3
"""Shared release version and platform contract."""
import argparse
import re

PLATFORMS = (('linux', 'amd64'), ('linux', 'arm64'), ('darwin', 'amd64'),
             ('darwin', 'arm64'), ('windows', 'amd64'))


def validate_version(version):
    if not re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?', version):
        raise ValueError('Invalid release version')
    if '-' in version:
        for part in version.split('-', 1)[1].split('.'):
            if part.isdigit() and len(part) > 1 and part.startswith('0'):
                raise ValueError('Numeric prerelease identifiers must not have leading zeros')
    return version


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('version')
    args = parser.parse_args()
    try:
        print(validate_version(args.version))
    except ValueError as error:
        parser.error(str(error))
