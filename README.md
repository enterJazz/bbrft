# BRFT

```
 ____  /\  ____   ____   _____  _____
| __ )|/\||___ \ |  _ \ |  ___||_   _|
|  _ \      __) || |_) || |_     | |
| |_) |    / __/ |  _ < |  _|    | |
|____/    |_____||_| \_\|_|      |_|
```

Best-RFT

## About
The Best Robust File Transfer Protocol (BRFTP) is an application- and transport level request/response protocol with minimal state, built on top of UDP. The protocol enables file transfer over an unreliable network. A server thereby offers files for download. A client may request one or more files for download. BRFTP enables the client to robustly retrieve files from the server, transfer in a compressed manner. Additionally, BRFTP enables file change detection, download resumption and multi-file downloads.

For a detailed overview, view the [BRFT RFC](bbrft-rfc.pdf) contained in this repo.

This instance was implemented by:
- Michael Hegel: michael.hegel@tum.de
- Wladislaw Meixner: wlad.meixner@tum.de
- Robert Schambach: scha@in.tum.de

## Installation

The brft client and server can be easily installed using the native go command

```bash
go install gitlab.lrz.de/ge82xiz/brft
```

## Usage
To view command and subcommand usage, use the `bbrft --help` command:
```
NAME:
   BRFTP - serve or fetch remote files

USAGE:
   BRFTP [global options] command [command options] [arguments...]

COMMANDS:
   serve, s  serve the files in a directory for download
   get, g    get resources from a BRFTP server
   help, h   Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --debug, -d  enables debug/test mode (default: false)
   --help, -h   show help (default: false)
```
The `--help` option may also be specified for the bbrft subcommands, e.g. `bbrft serve --help`
```
NAME:
   BRFTP serve - serve the files in a directory for download

USAGE:
   BRFTP serve [command options] [DIR] [PORT]

OPTIONS:
   --help, -h  show help (default: false)

```
Or, `bbrft get --help`:
```
NAME:
   BRFTP get - get resources from a BRFTP server

USAGE:
   BRFTP get command [command options] [arguments...]

COMMANDS:
     metadata, m  get metadata from a BRFTP server; specify no file name to list all available files, or specify a file name to get its size and checksum
     file, f      get FILE(s) from a BRFTP server at SERVER ADDRESS and store them in DOWNLOAD DIR; set CHECKSUM to assert file content's SHA-256 hash matches CHECKSUM
     help, h      Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)

```
The usage of the contained subcommands can again be viewed in detail with the `--help` flag.

### Running the server

To run the server, use the `bbrft serve` command. Please specify a folder from which bbrft should serve files for download as well which port bbrft should listen to for incoming connections. By convention, bbrft uses port 1337:
```bash
bbrft serve YOUR_FOLDER_TO_SHARE 1337
```

### Client

Files and corresponding Meta Data may be fetched with:
```bash
bbrft get [file|metadata]
```
#### MetaData Commands
To view which files a bbrft server offers for download, use the `get metadata` command, targeting the server of interest, without specifying a file name:
```bash
bbrft get metadata bbrft.example.com:1337
```
Further, to view extended metadata of a specific file, specify said file name in the `metadata` command:
```bash
bbrft get metadata my-file-of-interest.file bbrft.example.com:1337
```

#### File Request Commands
Finally, to download the file, use the `get file` command with a folder to download to as well as the target bbrft server:
```bash
bbrft file metadata my-file-of-interest.file /home/me/downloads bbrft.example.com:1337
```
If you wish to be sure of the file's contents, you may specify the file's hexadecimal-encoded sha256 checksum, obtainable via the MetaDataReq after the corresponding file, delimited with `:`:
```bash
bbrft file metadata my-file-of-interest.file:b958bc7b84c18196b0f548eca3a88a2ee7b1ce81231c22c231cc1af99fc36f6c /home/me/downloads bbrft.example.com:1337
```
To download multiple files, simply specify multiple files in the `get file` command; checksums can optionally be included:
```bash
bbrft file metadata my-file-of-interest.file:b958bc7b84c18196b0f548eca3a88a2ee7b1ce81231c22c231cc1af99fc36f6c mona-lisa.jpg top-gun-2.mp4:dcdf5ce055a53f714a45fd9d69c4a494e5282f9d0d0849202b3cb63823515a6c /home/me/downloads bbrft.example.com:1337
```