OPERATOR_NAME  := dmz-tainter
VERSION := $(shell date +%Y%m%d%H%M)
IMAGE := artifact-repository.intra.coop:9442/coop-integration/$(OPERATOR_NAME)

install_deps:
	dep ensure

build:
	rm -rf bin/%/$(OPERATOR_NAME)
	go build -v -i -o bin/$(OPERATOR_NAME) ./
	rm -rf bin/linux/$(OPERATOR_NAME)
	GOOS=linux GOARCH=amd64 go build -v -i -o bin/linux/$(OPERATOR_NAME) ./

build-image: bin/linux/$(OPERATOR_NAME)
	docker build . -t $(IMAGE):$(VERSION) -t $(IMAGE):latest
	
push-image: bin/linux/$(OPERATOR_NAME)
	docker push $(IMAGE)
	
deploy: build build-image push-image