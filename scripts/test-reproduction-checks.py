#!/usr/bin/env python3
"""Regression checks for wire startup and non-timing success-count verification."""
import csv
import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch

ROOT = Path(__file__).resolve().parents[1]

def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module

wire = load('wire', ROOT / 'implementation/experiments/scripts/run-wire-tls-validation.py')
claims = load('claims', ROOT / 'scripts/check-claims.py')

class WireChecks(unittest.TestCase):
    def server(self, failure=None):
        server = Mock()
        server.poll.return_value = 1 if failure else None
        server.communicate.return_value = ('', failure or '')
        return server

    def run_group(self, servers):
        with patch.object(wire, 'reserve_port', side_effect=[41001, 41002, 41003]), \
             patch.object(wire.time, 'sleep'), \
             patch.object(wire.subprocess, 'Popen', side_effect=servers) as popen, \
             patch.object(wire, 'run', return_value=(0, 'Server Temp Key: X25519')):
            result = wire.run_local_group('X25519', 1, Path('cert'), Path('key'))
            return result, popen

    def test_bind_conflict_retries_and_cleans_up(self):
        good = self.server()
        result, popen = self.run_group([self.server('Address already in use'), good])
        self.assertEqual(result[:3], ('passed', 'X25519', 1))
        self.assertEqual(popen.call_count, 2)
        self.assertIn('127.0.0.1:41002', popen.call_args.args[0])
        good.terminate.assert_called_once()

    def test_other_server_errors_are_not_retried(self):
        result, popen = self.run_group([self.server('unsupported group')])
        self.assertEqual(result[0], 'failed-server-start')
        self.assertEqual(popen.call_count, 1)

    def test_repeated_bind_failure_stays_a_failure(self):
        result, popen = self.run_group([self.server('Address already in use') for _ in range(3)])
        self.assertEqual(result[0], 'failed-server-start')
        self.assertEqual(popen.call_count, 3)

    def test_trial_counts_are_not_skipped(self):
        c32 = next(c for c in claims.CLAIMS if c['id'] == 'C32')
        original = claims.rows(claims.WT)
        for scenario in ('intact', 'failed', 'missing'):
            with self.subTest(scenario=scenario), tempfile.TemporaryDirectory() as tmp:
                rows = [dict(row) for row in original]
                if scenario == 'failed':
                    rows[1]['successes'] = '0'
                    rows[1]['status'] = 'failed-server-start'
                elif scenario == 'missing':
                    rows.pop()
                with (Path(tmp) / claims.WT).open('w', newline='') as out:
                    writer = csv.DictWriter(out, fieldnames=original[0].keys())
                    writer.writeheader()
                    writer.writerows(rows)
                with patch.object(claims, 'PROCESSED', Path(tmp)):
                    result, _ = claims.evaluate(c32, skip_timing=True)
                self.assertEqual(result, 'PASS' if scenario == 'intact' else 'FAIL')

if __name__ == '__main__':
    unittest.main()
