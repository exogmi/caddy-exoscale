import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from release_version import parse_tag, plan_version, validate_module_path


class VersionTests(unittest.TestCase):
    def test_initial_release(self):
        self.assertEqual(plan_version([], [], "initial commit"), ("v1.0.0", False))

    def test_patch_without_conventional_commit(self):
        self.assertEqual(plan_version(["v1.0.0"], [], "Update documentation"), ("v1.0.1", False))

    def test_numeric_order(self):
        self.assertEqual(plan_version(["v1.0.9", "v1.0.10", "v0.9.99"], [], "fix: bug"), ("v1.0.11", False))

    def test_features(self):
        self.assertEqual(plan_version(["v1.2.3"], [], "fix: bug\n\nfeat(dns): support new feature"), ("v1.3.0", False))

    def test_breaking_changes(self):
        for message in ["feat!: changed config", "fix(dns)!: new interface", "fix: change\n\nBREAKING CHANGE: old keys removed", "fix: change\n\nBREAKING-CHANGE: new API"]:
            with self.subTest(message=message):
                self.assertEqual(plan_version(["v1.2.3"], [], message), ("v2.0.0", False))

    def test_retry_reuses_version(self):
        self.assertEqual(plan_version(["v1.2.3"], ["v1.2.3"], "feat!: original message"), ("v1.2.3", True))

    def test_old_release_is_rejected(self):
        with self.assertRaises(ValueError):
            plan_version(["v1.0.0", "v1.0.1"], ["v1.0.0"], "")

    def test_non_release_tags_are_ignored(self):
        for tag in ["v1.0.0-beta.1", "latest", "1.0.0", "v01.0.0", "v1.0.0suffix"]:
            with self.subTest(tag=tag):
                self.assertIsNone(parse_tag(tag))
                self.assertEqual(plan_version([tag], [], ""), ("v1.0.0", False))

    def test_go_major_version_path(self):
        validate_module_path("v1.2.3", "github.com/exogmi/caddy-exoscale")
        validate_module_path("v2.0.0", "github.com/exogmi/caddy-exoscale/v2")
        validate_module_path("v10.0.0", "github.com/exogmi/caddy-exoscale/v10")
        for tag, module in [
            ("v2.0.0", "github.com/exogmi/caddy-exoscale"),
            ("v3.0.0", "github.com/exogmi/caddy-exoscale/v2"),
        ]:
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                validate_module_path(tag, module)

    def test_cli_with_real_git_history(self):
        script = Path(__file__).with_name("release_version.py").resolve()
        with tempfile.TemporaryDirectory() as directory:
            (Path(directory) / "go.mod").write_text("module github.com/exogmi/caddy-exoscale\n")
            def git(*args):
                return subprocess.check_output(["git", *args], cwd=directory, text=True, stderr=subprocess.DEVNULL)

            git("init", "-b", "main")
            git("config", "user.name", "Release Test")
            git("config", "user.email", "test@example.invalid")
            git("config", "commit.gpgsign", "false")
            git("commit", "--allow-empty", "-m", "Initial commit")
            git("tag", "v1.0.0")
            git("commit", "--allow-empty", "-m", "feat(dns): new option")
            output = Path(directory) / "outputs"
            env = dict(os.environ, GITHUB_OUTPUT=str(output))
            result = subprocess.check_output([sys.executable, str(script)], cwd=directory, env=env, text=True)
            self.assertIn("tag=v1.1.0", result)
            self.assertIn("reused=false", output.read_text())
            git("tag", "v1.1.0")
            result = subprocess.check_output([sys.executable, str(script)], cwd=directory, env=env, text=True)
            self.assertIn("reused=true", result)


if __name__ == "__main__":
    unittest.main()
