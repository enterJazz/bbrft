# TODO

## BRFT
- figure out when the conn&stream need to be locked
- graceful shutdown of connections
- adapt meta messages to the new Message interface
- create a log package and refactor logging 

- client-server tests
    - simple test for negotiation
        - cleanup the received file before starting the test
    - make sure the linking between server and client works correctly!
    - resumption
    - multiple concurrent downloads
    - resumption
    - stress test with A LOT of downloads

## MetaData
- Do we allow recursive directories on the server?


### Nice to haves
- add a maximum number of streams per connection/peer
- support for multiple files with the same name and different revision

## CLI
- add cli support for enabling/disabling compression

# Notes
- not so nice that we can not link a FileReq & FileResp, but have to rely on the order that responses are sent
    - added sessionID to FileReq
- we haven't specified message types for the metadata packets -> added some to the specification
- it would have been nicer for implementation to add a packet size to the brft header
- it would have been nice to have a chunk size as part of the normal negotiation instead of giving the server free reign
- we had no way to indicate if a MetaDataResp would be extended or not without holding more client state, making things messy
    -> we added a byte to the MetaResp to indicate whether the response is extended or not
