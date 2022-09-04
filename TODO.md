# TODO

## BRFT
- figure out when the conn&stream need to be locked
- I need to find a way not to overload the btp.Conn
    - control messages should always be preferred
    - create a separate routine that sends out packets, this way we also prevent different fragments being interleaved
- read of the whole message must happen sequentually
    - the main routine should also parse the messages
    - the parsed packet is handed/sent to the function that actually handles it
- graceful shutdown of connections
- adapt meta messages to the new Message interface

- I think it would be really nice to have a separate goroutine per stream
    - This could then coordinate that no messages is sent out-of-order/mutliple times
    - However, the in-order FileRequest and FileResponse seems to be a problem here

- client-server tests
    - simple test for negotiation
    - make sure the linking between server and client works correctly!
    - 

- add a maximum number of streams per connection/peer
- support for multiple files with the same name and different revision

## CLI
- add cli support for enabling/disabling compression

# Notes
- not so nice that we can not link a FileReq & FileResp, but have to rely on the order that responses are sent
    - added sessionID to FileReq
- we haven't specified message types for the metadata packets -> added some to the specification
- it would have been nicer for implementation to add a packet size to the brft header
