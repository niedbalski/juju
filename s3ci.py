#!/usr/bin/env python
from argparse import ArgumentParser
from ConfigParser import ConfigParser
import logging
import os
import re
import sys

from boto.s3.connection import S3Connection

from jujuci import (
    acquire_binary,
    JobNamer,
    PackageNamer,
    )
from download_juju import (
    _download_files,
    filter_keys,
    )
from utility import configure_logging


def parse_args(args=None):
    parser = ArgumentParser()
    subparsers = parser.add_subparsers(help='sub-command help', dest='command')
    parser_get_juju_bin = subparsers.add_parser(
        'get-juju-bin', help='Retrieve and extract juju binaries.')
    parser_get_juju_bin.add_argument('config')
    parser_get_juju_bin.add_argument('revision_build', type=int)
    parser_get_juju_bin.add_argument('workspace', nargs='?', default='.')
    parser_get_juju_bin.add_argument('--verbose', '-v', default=0,
                                     action='count')
    return parser.parse_args(args)


def get_s3_credentials(s3cfg_path):
    config = ConfigParser()
    config.read(s3cfg_path)
    access_key = config.get('default', 'access_key')
    secret_key = config.get('default', 'secret_key')
    return access_key, secret_key


def get_job_path(revision_build):
    namer = JobNamer.factory()
    job = namer.get_build_binary_job()
    return 'juju-ci/products/version-{}/{}'.format(revision_build, job)


class PackageNotFound(Exception):
    """Raised when a package cannot be found."""


def find_package_key(bucket, revision_build):
    prefix = get_job_path(revision_build)
    keys = bucket.list(prefix)
    suffix = PackageNamer.factory().get_release_package_suffix()
    filtered = [
        (k, f) for k, f in filter_keys(keys, suffix)
        if f.startswith('juju-core_')]
    if len(filtered) == 0:
        raise PackageNotFound('Package could not be found.')
    return sorted(filtered, key=lambda x: int(
        re.search(r'build-(\d+)/', x[0].name).group(1)))[-1]


def fetch_juju_binary(bucket, revision_build, workspace):
    package_key, filename = find_package_key(bucket, revision_build)
    logging.info('Selected: %s', package_key.name)
    _download_files([package_key], workspace)
    package_path = os.path.join(workspace, filename)
    logging.info('Extracting: %s', package_path)
    return acquire_binary(package_path, workspace)


def main():
    args = parse_args()
    log_level = logging.WARNING - args.verbose * (
        logging.WARNING - logging.INFO)
    configure_logging(log_level)
    if args.command == 'get-juju-bin':
        return get_juju_bin(args)
    else:
        raise Exception('{} not implemented.'.format(args.command))


def get_juju_bin(args):
    credentials = get_s3_credentials(args.config)
    conn = S3Connection(*credentials)
    bucket = conn.get_bucket('juju-qa-data')
    print(fetch_juju_binary(bucket, args.revision_build, args.workspace))


if __name__ == '__main__':
    sys.exit(main())
