# The build context is src/go, not the repository root.
#
# Railway builds this service with its root directory set to src/go, so nothing
# above this folder is in the context and paths like src/data.json cannot
# resolve. A root-context version of this file was merged on 2026-08-14 and the
# deploy failed with `"/src/data.json": not found`. If you want the root-context
# layout back, change the service's root directory to / in the Railway dashboard
# first, then change this file and docker-compose.yml together.
#
# data.json is the sibling copy in src/go. TestBothScriptCopiesAreIdentical keeps
# it byte-identical to src/data.json, which is what the frontend bundles.
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/multibot .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/multibot ./multibot
COPY --from=build /src/data.json ./data.json
EXPOSE 8080
ENTRYPOINT ["./multibot"]
