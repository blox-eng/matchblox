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
	xcodebuild \
		-scheme $(SCHEME) \
		-destination '$(DESTINATION)' \
		-derivedDataPath $(DERIVED) \
		-skipPackagePluginValidation \
		CODE_SIGNING_ALLOWED=NO \
		build

.PHONY: icon
icon: ## Render the app icon from AppIcon.svg (needs rsvg-convert: brew install librsvg)
	@command -v rsvg-convert >/dev/null 2>&1 || { echo "rsvg-convert not found. Run: brew install librsvg"; exit 1; }
	mkdir -p $(DERIVED)
	rsvg-convert -w 1024 -h 1024 AppIcon.svg -o $(DERIVED)/AppIcon.png
	sips -s format jpeg -s formatOptions 100 $(DERIVED)/AppIcon.png --out $(DERIVED)/AppIcon.jpg >/dev/null
	sips -s format png $(DERIVED)/AppIcon.jpg --out Matchblox/Assets.xcassets/AppIcon.appiconset/AppIcon.png >/dev/null

.PHONY: test
test: ## Run the MatchbloxKit tests
	swift test --package-path MatchbloxKit
