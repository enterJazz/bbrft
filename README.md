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

## Installation

The brft client and server can be easily installed using the native go command

```bash
go install gitlab.lrz.de/ge82xiz/brft
```

## Usage

### Running the server

```bash
bbrft serve YOUR_FOLDER_TO_SHARE 1337
```

### Client

#### Requesting server file list

```bash
bbrft get metadata [SERVER_IP]:[SERVER_PORT]
```

#### Downloading file from server

```bash
bbrft get file [FILENAME] [DOWNLOAD_FOLDER] [SERVER_IP]:[SERVER_PORT]
```
