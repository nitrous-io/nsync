NSYNC
=====

## API
1. GET `/ping`:
2. POST `/containers/{slug}/tunnel/start`: Start port forwarding for given container.
  - Parameters:
     - `ports`: comma(`,`) separated. i.e) 3000-3002,5000
3. POST `/containers/{slug}/tunnel/stop`: Stop port forwarding for given container.
4. POST `/containers/{slug}/sync/start`: Start syncing for given container.
  - Parameters:
    - `remote_path`: remote path for sync.
5. POST `/containers/{slug}/sync/stop`: Stop syncing for given container.
6. POST `/subscribe`: Subscribe to container events.
7. GET `/debug/pprof/{path:.*}`: For debugging.
8. POST `/init`: This should be called first to set initial data.
  - Parameters:
    - `token`: remote path for sync.
    - `path`: path to sync in local.
    - `host`: nitrous host for api call.

- - -
Copyright (c) 2015-Present Nitrous, Inc. All Rights Reserved.
