FROM golang:1.19-alpine
COPY . /ossproject
WORKDIR /ossproject
ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    max_size=102400
RUN echo 'max_size=' $max_size
WORKDIR ./common
RUN go mod download

WORKDIR ../holder
RUN go mod download

RUN chmod -R 777 ./oss_start.sh
CMD /bin/sh ./oss_start.sh 0 $max_size


