FROM golang:1.22-alpine AS build
WORKDIR /src
COPY src/go/go.mod src/go/go.sum ./
RUN go mod download
COPY src/go/. .
COPY src/data.json ./data.json
RUN CGO_ENABLED=0 go build -o /out/multibot .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/multibot ./multibot
COPY --from=build /src/data.json ./data.json
EXPOSE 8080
ENTRYPOINT ["./multibot"]
