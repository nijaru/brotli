.PHONY: fmt
fmt:
	goimports -w .
	golines --base-formatter=gofumpt -w .
