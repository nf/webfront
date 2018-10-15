# Building...

FROM          golang:alpine as server-builder

WORKDIR       /webfront

RUN           apk update && apk upgrade && \
              apk add --no-cache bash git openssh build-base
ADD           . .
RUN           go get -d -v && \
              go test ./... && \
              go build

# Running...

FROM          alpine

WORKDIR       /app

RUN           apk update && apk add ca-certificates libcap

COPY          --from=server-builder /webfront/webfront /app
COPY          --from=server-builder /webfront/dev_certificates /app/dev_certificates
COPY          --from=server-builder /webfront/rules.json /app/rules.json

RUN           setcap cap_net_bind_service=+ep webfront

ENTRYPOINT    [ "./webfront"]