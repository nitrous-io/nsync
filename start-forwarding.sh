#!/bin/bash

curl -X POST localhost:64876/containers/nsync-test-141337/tunnel/start -d 'sshUrl=ssh://nitrous@pete-4644.nitrouspro.com:32866' -d 'ports=3000,5000'
