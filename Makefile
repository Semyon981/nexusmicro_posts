IN_FOLDER=cmd
OUT_FOLDER=build
IN_FILE=main.go
OUT_FILE=bin
ARCH_BUILD=amd64
OS_BUILD=linux


port-forward:
	kubectl port-forward service/nexususers-service 50052:50051 & \
	kubectl port-forward service/nexusstorage-service 50053:50051 & \
	kubectl port-forward service/nexuslinkedacc-service 50054:50051 & \
	kubectl port-forward service/cassandra 9042:9042



all: git subpull gen


gen:
	mkdir -p ./pb
	protoc --go_out=./ \
		--go-grpc_out=require_unimplemented_servers=false:./ \
		--proto_path=./nexusmicro_proto \
		./nexusmicro_proto/*.proto
	protoc-go-inject-tag -input=./pb/*


docker:
	docker-compose build
	docker-compose up


build:
	mkdir -p ${OUT_FOLDER}
	CGO_ENABLED=0 GOOS=${OS_BUILD} GOARCH=${ARCH_BUILD} go build -v -o ${OUT_FOLDER}/${OUT_FILE} ${IN_FOLDER}/${IN_FILE}


evans:
	evans --proto nexusmicro_proto/posts.proto


subpull:
	git submodule update --remote


.PHONY: build