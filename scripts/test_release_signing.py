"""Exercise real GPG signing and reject damaged or unexpected release inputs."""
import hashlib
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import zipfile

from release_common import PLATFORMS, validate_version

SIGNER = Path(__file__).with_name('sign-release.py')


class ReleaseSigningTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.keyring = tempfile.TemporaryDirectory(prefix='arin-test-gpg-')
        cls.env = dict(os.environ, GNUPGHOME=cls.keyring.name)
        cls.password = 'disposable-test-passphrase'
        subprocess.run(['gpg', '--batch', '--pinentry-mode', 'loopback', '--passphrase-fd', '0',
                        '--quick-generate-key', 'Disposable Release Test <test@example.invalid>',
                        'rsa2048', 'sign', '1d'], input=cls.password.encode(), env=cls.env,
                       check=True, capture_output=True)
        listing = subprocess.check_output(['gpg', '--with-colons', '--list-secret-keys'], env=cls.env, text=True)
        cls.fingerprint = next(line.split(':')[9] for line in listing.splitlines() if line.startswith('fpr:'))
        secret = subprocess.check_output(['gpg', '--batch', '--pinentry-mode', 'loopback',
                                          '--passphrase-fd', '0', '--armor', '--export-secret-keys', cls.fingerprint],
                                         input=cls.password.encode(), env=cls.env).decode()
        cls.signenv = dict(cls.env, GPG_PRIVATE_KEY=secret, PASSPHRASE=cls.password,
                          GPG_FINGERPRINT=cls.fingerprint)

    @classmethod
    def tearDownClass(cls):
        subprocess.run(['gpgconf', '--kill', 'gpg-agent'], env=cls.env, check=False)
        cls.keyring.cleanup()

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='arin-sign-test-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.manifest = self.root / 'terraform-provider-arin_0.0.0-test_manifest.json'
        self.manifest.write_text('{"version":1,"metadata":{"protocol_versions":["6.0"]}}\n')
        self.sums = self.root / 'terraform-provider-arin_0.0.0-test_SHA256SUMS'
        for system, arch in PLATFORMS:
            path = self.root / f'terraform-provider-arin_0.0.0-test_{system}_{arch}.zip'
            with zipfile.ZipFile(path, 'w') as archive:
                archive.writestr('disposable-test-binary', b'test fixture')
        self.write_checksums()

    def write_checksums(self):
        self.sums.write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + p.name + '\n'
                                    for p in sorted(self.root.iterdir()) if p != self.sums))

    def sign(self, env=None):
        return subprocess.run([sys.executable, str(SIGNER), str(self.root)],
                              env=env or self.signenv, capture_output=True, text=True)

    def test_signature_and_post_sign_tampering(self):
        result = self.sign()
        self.assertEqual(result.returncode, 0, result.stderr)
        signature = str(self.sums) + '.sig'
        verify = ['gpg', '--batch', '--verify', signature, str(self.sums)]
        self.assertEqual(subprocess.run(verify, env=self.env, capture_output=True).returncode, 0)
        self.assertFalse(Path(signature).read_bytes().startswith(b'-----BEGIN'))
        self.sums.write_text(self.sums.read_text() + 'tampered\n')
        self.assertNotEqual(subprocess.run(verify, env=self.env, capture_output=True).returncode, 0)

    def test_checksum_mismatch(self):
        self.manifest.write_text('tampered')
        self.assertNotEqual(self.sign().returncode, 0)
        self.assertFalse(Path(str(self.sums) + '.sig').exists())

    def test_extra_files_and_incomplete_release(self):
        for name in ('unexpected.txt', 'INCOMPLETE'):
            with self.subTest(name=name):
                p = self.root / name
                p.write_text('unexpected')
                self.assertNotEqual(self.sign().returncode, 0)
                p.unlink()

    def test_missing_platform_archive(self):
        next(self.root.glob('*linux_amd64.zip')).unlink()
        self.write_checksums()
        self.assertNotEqual(self.sign().returncode, 0)
        self.assertFalse(Path(str(self.sums) + '.sig').exists())

    def test_manifest_without_archives(self):
        for path in self.root.glob('*.zip'):
            path.unlink()
        self.write_checksums()
        self.assertNotEqual(self.sign().returncode, 0)
        self.assertFalse(Path(str(self.sums) + '.sig').exists())

    def test_wrong_fingerprint(self):
        env = dict(self.signenv, GPG_FINGERPRINT='0' * 40)
        self.assertNotEqual(self.sign(env).returncode, 0)
        self.assertFalse(Path(str(self.sums) + '.sig').exists())

    def test_parent_path_in_manifest(self):
        self.sums.write_text('0' * 64 + '  ../outside\n')
        self.assertNotEqual(self.sign().returncode, 0)


class ReleaseVersionTests(unittest.TestCase):
    def test_valid_versions(self):
        for version in ('0.1.0', '0.1.0-alpha.1', '1.2.3-0', '1.2.3-01a', '1.2.3-alpha-01'):
            with self.subTest(version=version):
                self.assertEqual(validate_version(version), version)

    def test_invalid_versions(self):
        for version in ('1.2.3-01', '1.2.3-alpha.01', '01.2.3', 'v1.2.3', '1.2.3-',
                        '1.2.3-alpha..1', '../1.2.3', '1.2.3\n'):
            with self.subTest(version=version):
                with self.assertRaises(ValueError):
                    validate_version(version)


if __name__ == '__main__':
    unittest.main()
