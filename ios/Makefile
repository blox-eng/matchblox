SCHEME ?= Matchblox
DESTINATION ?= generic/platform=iOS Simulator
DERIVED ?= build

.PHONY: install
install: ## Install XcodeGen with Homebrew
	@command -v brew >/dev/null 2>&1 || { echo "Homebrew is required. Install it from https://brew.sh"; exit 1; }
	@command -v xcodegen >/dev/null 2>&1 || brew install xcodegen

.PHONY: gen
gen: ## Generate Matchblox.xcodeproj from project.yml
	@command -v xcodegen >/dev/null 2>&1 || { echo "xcodegen not found. Run: make install"; exit 1; }
	xcodegen generate

.PHONY: build
build: gen ## Compile the app for the simulator, without code signing
	set -o pipefail; xcodebuild \
		-scheme $(SCHEME) \
		-destination '$(DESTINATION)' \
		-derivedDataPath $(DERIVED) \
		CODE_SIGNING_ALLOWED=NO \
		build

.PHONY: test
test: ## Run the MatchbloxKit tests
	swift test --package-path MatchbloxKit
