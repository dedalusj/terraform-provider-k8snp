default: testacc

.PHONY: build
build:
	go build -o .tmp/bin/terraform-provider-k8snp

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m
