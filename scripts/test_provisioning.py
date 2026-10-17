"""Render the actual proxy template and exercise Docker discovery without a server."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

from jinja2 import Environment, StrictUndefined
import yaml


ROOT = Path(__file__).resolve().parents[1]


def jinja():
    env = Environment(undefined=StrictUndefined)
    env.filters['to_json'] = json.dumps
    return env


class ProvisioningTests(unittest.TestCase):
    @unittest.skipUnless(shutil.which('docker'), 'Docker CLI required')
    def test_rendered_proxy_matches_application_routes_and_persists_certificates(self):
        defaults = yaml.safe_load((ROOT / 'ansible/vars/production.yml').read_text())
        rendered = jinja().from_string(
            (ROOT / 'ansible/templates/traefik-compose.yml.j2').read_text()
        ).render(traefik_image=defaults['traefik_image'], acme_email='deploy+ci@example.com')
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'compose.yml'
            path.write_text(rendered)
            result = subprocess.run(['docker', 'compose', '-p', 'todo-edge', '-f', str(path),
                                     'config', '--format', 'json'],
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
        config = json.loads(result.stdout)
        proxy = config['services']['traefik']
        options = dict(value[2:].split('=', 1) for value in proxy['command'])
        app = yaml.safe_load((ROOT / 'docker-compose.yml').read_text(encoding='utf-8'))
        for name in ('backend', 'frontend'):
            labels = dict(value.split('=', 1) for value in app['services'][name]['labels'])
            self.assertEqual(labels['traefik.docker.network'], config['networks']['traefik']['name'])
            for key, value in labels.items():
                if key.endswith('.entrypoints'):
                    self.assertEqual(options[f'entrypoints.{value}.address'], ':443')
                if key.endswith('.tls.certResolver'):
                    self.assertEqual(options[f'certificatesresolvers.{value}.acme.httpchallenge.entrypoint'], 'web')
        self.assertEqual(options['certificatesresolvers.default.acme.email'], 'deploy+ci@example.com')
        self.assertEqual(options['providers.docker.exposedbydefault'], 'false')
        self.assertEqual({int(p['published']) for p in proxy['ports']}, {80, 443})
        certificate_mount = next(v for v in proxy['volumes'] if v['target'] == '/letsencrypt')
        self.assertEqual(certificate_mount['type'], 'volume')
        self.assertIn(certificate_mount['source'], config['volumes'])
        self.assertTrue(config['networks']['traefik']['external'])

    @unittest.skipIf(os.name == 'nt', 'Discovery runs on the Linux Ansible target')
    def test_discovery_reuses_official_proxy_and_ignores_unrelated_containers(self):
        tasks = yaml.safe_load((ROOT / 'ansible/tasks/proxy.yml').read_text())
        cmd = jinja().from_string(tasks[0]['ansible.builtin.shell']['cmd']).render()
        with tempfile.TemporaryDirectory() as folder:
            docker = Path(folder) / 'docker'
            docker.write_text('#!/bin/sh\nprintf "%s\\n" "$DOCKER_ROWS"\nexit "$DOCKER_RC"\n')
            docker.chmod(0o755)
            env = dict(os.environ, PATH=folder + os.pathsep + os.environ['PATH'], DOCKER_RC='0')
            rows = [{'ID': 'proxy', 'Image': 'docker.io/library/traefik:v3.7'},
                    {'ID': 'app', 'Image': 'ghcr.io/example/backend:sha'},
                    {'ID': 'fake', 'Image': 'example/traefik:v3'}]
            env['DOCKER_ROWS'] = '\n'.join(json.dumps(row) for row in rows)
            result = subprocess.run(['bash', '-c', cmd], env=env, capture_output=True, text=True, check=True)
            self.assertEqual(json.loads(result.stdout), ['proxy'])
            env['DOCKER_ROWS'] = ''
            result = subprocess.run(['bash', '-c', cmd], env=env, capture_output=True, text=True, check=True)
            self.assertEqual(json.loads(result.stdout), [])
            # A failed Docker query must never look like an empty server to provision.
            env['DOCKER_RC'] = '1'
            result = subprocess.run(['bash', '-c', cmd], env=env, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)


if __name__ == '__main__':
    unittest.main()
