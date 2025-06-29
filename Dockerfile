FROM golang:alpine as build
ENV GO111MODULE=on

COPY . /workdir
WORKDIR /workdir
RUN go build

FROM alpine:latest
COPY --from=build /workdir/control-remo-from-pi/ /usr/local/bin
CMD ["control-remo-from-pi"]
LABEL org.opencontainers.image.source=https://github.com/Eivy/control-remo-from-pi
