FROM golang:latest AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

# CGO has to be disabled for alpine
RUN go build -o dist

##
## Deploy
##
FROM debian:latest

WORKDIR /

COPY --from=build /app/dist brft

EXPOSE 1337
ENTRYPOINT ["/brft", "serve", "/opt/files", "1337"]