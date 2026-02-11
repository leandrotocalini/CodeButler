Build and install CodeButler with version from VERSION file:

1. Read the VERSION file to get the version number
2. Run `go build -ldflags "-X main.Version=$(cat VERSION | tr -d '[:space:]')" -o codebutler ./cmd/codebutler/`
3. Run `go install -ldflags "-X main.Version=$(cat VERSION | tr -d '[:space:]')" ./cmd/codebutler/`
4. Confirm both succeeded and print the version installed
