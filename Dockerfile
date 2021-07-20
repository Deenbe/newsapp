FROM golang:1.16.3-alpine3.13

RUN mkdir -p /home/posts

WORKDIR /home/posts
COPY go.mod /home/posts/
COPY go.sum /home/posts/
COPY main.go /home/posts/

ENV GOPROXY=https://goproxy.io,direct

RUN go get
RUN go build -o build/posts .


FROM alpine:3.13
COPY --from=0 /home/posts/build/posts /usr/local/bin/
RUN apk add --update curl && \
    rm -rf /var/cache/apk/*

HEALTHCHECK --interval=30s --timeout=1s --retries=3 CMD curl http://localhost:8080/healthcheck || exit 1

ENTRYPOINT ["posts", "-port", "8080"]
