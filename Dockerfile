

FROM golang:1.20 AS builder

ARG GH_USER_NAME
ARG GH_BUILD_TOKEN

###################### DATABASE SETUP #######################

WORKDIR /src

# Copy project files
ADD . /src

# Download dependencies
# NOTE: we have to do this the slow way of rebuilding from scratch each time
# because creating a dependency graph and caching the built dependencies triggers
# a panic in the `go get` command
RUN git config --global url."https://${GH_USER_NAME}:${GH_BUILD_TOKEN}@github.com".insteadOf "https://github.com" \
    && export GOPRIVATE="github.com/gage-technologies" \
    && /usr/local/go/bin/go get ./...

RUN /usr/local/go/bin/go generate && /usr/local/go/bin/go build -o /tmp/gigo-ws .

############################################################

FROM golang:1.20

COPY --from=builder /tmp/gigo-ws /bin

RUN mkdir -p /logs \
    && mkdir -p /keys \
    && mkdir -p /gigo-core \
    && mkdir -p /db-files

ENV NAME GIGO
ENV TZ "America/Chicago"
ENTRYPOINT ["/bin/gigo-ws"]
