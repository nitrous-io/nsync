#!/bin/bash

curl -X POST localhost:64876/containers/nsync-test-141337/sync/start -d 'sshUrl=ssh://nitrous@pete-4644.nitrouspro.com:32866' -d 'remotePath=code'
