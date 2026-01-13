FROM golang:alpine as build
ENV GO111MODULE=on

COPY . /workdir
WORKDIR /workdir
RUN go build -ldflags="-s -w" -trimpath -o control-remo cmd/control-remo/main.go

FROM alpine:latest
COPY --from=build /workdir/control-remo/ /usr/local/bin
CMD ["control-remo"]
LABEL org.opencontainers.image.source=https://github.com/Eivy/control-remo-from-pi
