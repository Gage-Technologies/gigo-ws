ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
include .env

protos:
	# maybe don't install latest of these but for now it works
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install storj.io/drpc/cmd/protoc-gen-go-drpc@latest
	mkdir -p ${ROOT_DIR}/protos/ws
	cd ${ROOT_DIR}/ws-protos && git pull
	protoc --go_out=${ROOT_DIR} --go-drpc_out=${ROOT_DIR} --proto_path=${ROOT_DIR}/ws-protos ${ROOT_DIR}/ws-protos/**

agent:
	mkdir -p ${ROOT_DIR}/bin
	cd ${ROOT_DIR} && go mod tidy
	go build -o ${ROOT_DIR}/bin/agent ${ROOT_DIR}/coder/agent

cli:
	mkdir -p ${ROOT_DIR}/bin
	cd ${ROOT_DIR} && go mod tidy
	CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o ${ROOT_DIR}/bin/gigo-ws-cli ${ROOT_DIR}/cli

docker:
	docker build \
		--build-arg "GH_BUILD_TOKEN=${GITHUB_BUILD_TOKEN}" \
		--build-arg "GH_USER_NAME=${GITHUB_USER_NAME}" \
		-t ${DOCKER_IMAGE} .

docker-push:
	docker push ${DOCKER_IMAGE}
