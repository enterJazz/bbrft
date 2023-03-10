# TODO
- write a readme
- improve the comments

@robert
- close btp
- cli extra files DONE
- fix metadatareq
- maybe README
- add checksum tests

## BRFT
- graceful shutdown of connections (michi)
    - the btp layer does not transmitt the brft.close packet, because the connection is closed immediately
- ListFileMetaData must return multiple MetaDataResps
    - therefore, they also need to be concatenated


- client-server tests
    - simple test for negotiation
        - cleanup the received file before starting the test
    - make sure the linking between server and client works correctly!
    - resumption
    - multiple concurrent downloads
    - connection migration
    - stress test with A LOT of downloads
    - metadatareq/resp with more than 255 items
    - multiple concurrent downloads over multiple connections

## Requirements
- must be able to recover from connection drops
- command line: allow multiple files for concurrent download

## MetaData
- Do we allow recursive directories on the server?
    - not defined in specs -> no
- wait for 255 following values @robert

### Nice to haves
- add a maximum number of streams per connection/peer
- support for multiple files with the same name and different revision
- create a log package and refactor logging

## CLI
- add cli support for enabling/disabling compression
- add cli support for multiple files (in devel branch) @robert DONE
- add logger customization

# Notes
- not so nice that we can not link a FileReq & FileResp, but have to rely on the order that responses are sent
    - added sessionID to FileReq
- we haven't specified message types for the metadata packets -> added some to the specification
- it would have been nicer for implementation to add a packet size to the brft header
- it would have been nice to have a chunk size as part of the normal negotiation instead of giving the server free reign
- we had no way to indicate if a MetaDataResp would be extended or not without holding more client state, making things messy
    -> we added a byte to the MetaResp to indicate whether the response is extended or not
- didn't do multiple versions of files
- the server does not read in all the checksums once, but rather computes on demand