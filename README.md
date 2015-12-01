NSYNC
=====

## API
1. GET `/ping`.
2. POST `/containers/{slug}/tunnel/start`: Start port forwarding for given container.
  - Parameters:
     - `ports`: comma(`,`) separated.
3. POST `/containers/{slug}/tunnel/stop`: Stop port forwarding for given container.
4. POST `/containers/{slug}/sync/start`: Start syncing for given container.
  - Parameters:
    - `remotePath`: remote path for sync.
5. POST `/containers/{slug}/sync/stop`: Stop syncing for given container.
6. POST `/subscribe`: Subscribe to receive message from server.
7. GET `/debug/pprof/{path:.*}`: For debugging.

- - -
Copyright (c) 2015-Present Nitrous, Inc. All Rights Reserved.
