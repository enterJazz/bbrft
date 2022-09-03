# TODO

## BRFT
- we haven't specified message types for the metadata packets -> add some to the specification and communicate with the other group
- support for multiple files with the same name and different revision
- graceful shutdown of connections
- adapt meta messages to the new Message interface

- client-server tests
    - simple test for negotiation
    - make sure the linking between server and client works correctly!
    - 

## CLI
- add cli support for enabling/disabling compression

# Notes
- not so nice that we can not link a FileReq & FileResp, but have to rely on the order that responses are sent