#!/usr/bin/env python3
"""Tests for the scorecard gate in run.py.

A CI gate that cannot fail is worse than no gate — it reads as a guarantee and
provides none. These check that each condition in check() actually trips, and
that the honest documented miss does not.

    python3 -m unittest discover -s eval -p '*_test.py'
"""
import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import run  # noqa: E402

BASELINE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "expected.json")


def scorecard(**over):
    """A passing scorecard, shaped exactly like summarize() produces."""
    s = {
        "malicious_total": 7,
        "malicious_caught": 6,
        "benign_total": 3,
        "false_blocks": 0,
        "false_quarantine_rate": 0.0,
        "catches": 6,
        "static_blind_catches": 5,
        "misses": ["@acme/calc-mcp"],
        "false_block_examples": [],
        "rows": [{"name": "@acme/calc-mcp"}, {"name": "@acme/notes-mcp"}],
    }
    s.update(over)
    return s


class CheckTest(unittest.TestCase):
    def assertProblem(self, s, needle):
        problems = run.check(s, BASELINE)
        self.assertTrue(problems, f"expected a failure mentioning {needle!r}, got none")
        self.assertTrue(
            any(needle in p for p in problems),
            f"expected a failure mentioning {needle!r}, got {problems}")

    def test_documented_miss_passes(self):
        self.assertEqual(run.check(scorecard(), BASELINE), [])

    def test_a_quarantined_benign_artifact_fails(self):
        self.assertProblem(
            scorecard(false_blocks=1, false_quarantine_rate=0.3333,
                      false_block_examples=["@acme/echo-mcp"]),
            "false-quarantine")

    def test_a_new_miss_fails(self):
        self.assertProblem(
            scorecard(misses=["@acme/calc-mcp", "@acme/notes-mcp"], malicious_caught=5),
            "@acme/notes-mcp")

    def test_a_fixed_miss_fails_until_the_baseline_is_updated(self):
        # Good news still has to be recorded, or the accepted-miss list becomes
        # a place to quietly park regressions.
        self.assertProblem(scorecard(misses=[], malicious_caught=7), "caught now")

    def test_losing_the_static_blind_contrast_fails(self):
        self.assertProblem(scorecard(static_blind_catches=0), "static-blind")

    def test_an_empty_run_fails_instead_of_passing_vacuously(self):
        # The failure mode that matters: the engine comes up with no zoo, every
        # denominator is 0, and every rate reads as a perfect score.
        empty = scorecard(malicious_total=0, malicious_caught=0, benign_total=0,
                          catches=0, static_blind_catches=0, misses=[],
                          false_quarantine_rate=None, rows=[])
        problems = run.check(empty, BASELINE)
        self.assertTrue(any("zoo did not load" in p for p in problems), problems)
        self.assertTrue(any("benign controls" in p for p in problems), problems)

    def test_baseline_file_is_well_formed(self):
        with open(BASELINE) as f:
            want = json.load(f)
        self.assertIsInstance(want.get("accepted_misses"), dict)
        for name, reason in want["accepted_misses"].items():
            self.assertGreater(len(reason), 40, f"{name} is accepted without a real reason")


class ArgParsingTest(unittest.TestCase):
    def test_out_accepts_both_spellings(self):
        with tempfile.TemporaryDirectory() as d:
            for argv in ([".", "--out", d], [".", f"--out={d}"]):
                sys.argv = argv
                self.assertEqual(run.arg_value("--out"), d)
        sys.argv = ["."]
        self.assertEqual(run.arg_value("--out", "fallback"), "fallback")


if __name__ == "__main__":
    unittest.main()
