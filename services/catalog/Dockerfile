FROM --platform=$BUILDPLATFORM golang:1.23.4-alpine

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

RUN go build -o catalog main.go

EXPOSE 8080
CMD ["./catalog"]