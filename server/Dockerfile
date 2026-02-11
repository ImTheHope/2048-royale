FROM golang:1.21-alpine
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
COPY index.html .
EXPOSE 8080
RUN go build -o server .
CMD ["./server"]