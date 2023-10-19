FROM alpine:latest

WORKDIR /usr/src/app

COPY ./build/bin ./bin

EXPOSE 50051

CMD ["./bin"]