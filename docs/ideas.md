## Structure (WIP)

- Two layer setup
- Transfer progress is handled in bytes
- Ordering is handled lock step or seq number
- multifile transfer: compressed into larger file
- both directions: start server on client and vice versa
- multiple file transfer: use multiple L1 instances on multiple ports L2 must associate packages to the right file transfer

1. Layer 1 (Best Transport Layer) 
    - Congestion control
    - flow control
    - ack and replay
    - no checksum for framgments -> since provided by UDP
    - connection timeout
    - add cancel from client to server
    - lock step start improve down the line (also add sequence numbers)

2. Layer 2 (File Transfer Layer)
    - resumption
    - file checksum
    - (optional) auth
    - (optional) compression
    - (optional) encryption

## Errors

- File Not Found: Not found packet response
- File Changed in transport: Fail (can file be locked? or duplicated for transfer)
- File Changes at interruption and resumption: offset bytes
- No space left on device: send L1 cancel frame
- Ctrl-C -> send cancel frame
- Client IP address changes: same as interruption and resumption

## Requirements

1. Point-to-Point maybe use some network simulation to test different network structures
2. resumption MUST send new IP and MAY listen to system events to detect network changes
3. Flow control: TBD
4. Congestion Controll: TBD for start AIMD
